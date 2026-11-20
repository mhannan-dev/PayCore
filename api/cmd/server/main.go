package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"paycore/api/internal/config"
	"paycore/api/internal/database"
	"paycore/api/internal/handler"
	"paycore/api/internal/helpers"
	"paycore/api/internal/mailer"
	"paycore/api/internal/mfa"
	"paycore/api/internal/repository"
	"paycore/api/internal/server"
	"paycore/api/internal/service"
	"paycore/api/internal/storage"
	"paycore/api/migrations"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	listRoutes := flag.Bool("routes", false, "Print all registered API endpoints and exit")
	flag.Parse()

	cfg := config.Load()
	var db server.DatabasePinger
	// Composition root: the only place that creates concrete repositories, services
	// and handlers and wires them together (docs/coding_rules.md rule 25).
	var handlers server.Handlers
	var emailCancel context.CancelFunc
	var emailDone chan struct{}
	if !*listRoutes {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		pool, err := database.Open(ctx, cfg.DatabaseURL)
		cancel()
		if err != nil {
			// Connection errors can contain credentials; do not log the raw DSN/error.
			log.Fatal("PostgreSQL connection failed. Check DATABASE_URL and database availability.")
		}
		defer pool.Close()
		migrationCtx, migrationCancel := context.WithTimeout(context.Background(), 20*time.Second)
		err = migrations.Apply(migrationCtx, pool)
		migrationCancel()
		if err != nil {
			log.Fatal("Database migration failed; API startup stopped.")
		}
		challenges := service.NewCaptchaStore()
		authenticator, err := mfa.New(cfg.MFAEncryptionKey)
		if err != nil {
			log.Fatal("MFA_ENCRYPTION_KEY is missing or invalid. Run go run ./cmd/mfa-key from api, or configure a base64-encoded 32-byte key.")
		}
		mfaService := service.NewMFAServiceWithMFAPolicy(repository.NewMFARepository(pool), authenticator, cfg.MFAEnabled)
		handlers.MFA = handler.NewMFAHandler(mfaService)
		authService, err := service.NewAuthServiceWithMFAPolicy(repository.NewUserRepositoryWithMFAPolicy(pool, cfg.MFAEnabled), challenges, mfaService, cfg.MFAEnabled)
		if err != nil {
			log.Fatal("Authentication initialization failed.")
		}
		auditLog := repository.NewAuditRepository(pool)
		mfaService.Audit = auditLog
		authService.Audit = auditLog
		guard := handler.NewGuard(authService)
		handlers.Security = handler.NewSecurityHandler(service.NewSecurityServiceWithMFAPolicy(repository.NewSecurityRepositoryWithMFAPolicy(pool, cfg.MFAEnabled), authenticator, cfg.MFAEnabled), guard)
		handlers.Auth = handler.NewAuthHandler(authService, challenges)
		handlers.Audit = handler.NewAuditHandler(auditLog, guard)
		files, err := storage.OpenLocal(cfg.UploadDir)
		if err != nil {
			log.Fatal("Upload storage initialization failed. Check UPLOAD_DIR permissions.")
		}
		defer files.Close()
		attachments := repository.NewAttachmentRepository(pool)
		attachmentService := service.NewAttachmentService(attachments, files, helpers.NewWebPConverter(2))
		handlers.Attachments = handler.NewAttachmentHandler(guard, attachmentService, service.NewPhotoService(attachments, files))
		handlers.Matching = handler.NewMatchingHandler(service.NewMatchingService(repository.NewMatchingRepository(pool)), guard)
		handlers.Statements = handler.NewStatementHandler(service.NewStatementService(attachmentService, repository.NewLedgerRepository(pool), repository.NewStatementRepository(pool)), guard)
		var mail mailer.Sender
		switch {
		case cfg.SMTPHost != "":
			smtpSender := mailer.SMTP{Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUsername, Password: cfg.SMTPPassword, From: cfg.SMTPFrom, Encryption: cfg.SMTPEncryption}
			if err := smtpSender.Validate(); err != nil {
				log.Fatalf("Email configuration (%s) is invalid: %v", cfg.SMTPSource, err)
			}
			mail = smtpSender
			slog.Info("smtp_configured", "host", cfg.SMTPHost, "port", cfg.SMTPPort, "encryption", cfg.SMTPEncryption, "authenticated", cfg.SMTPUsername != "", "source", cfg.SMTPSource)
		case cfg.Environment == "development":
			mail = mailer.Log{}
			log.Println("Email: SMTP_HOST is not set; invitation emails are written to this log (development only)")
		default:
			log.Println("Email: SMTP_HOST is not set; invitations are created but not emailed")
		}
		queue := service.NewEmailWorker(repository.NewEmailOutboxRepository(pool), mail)
		workerCtx, workerCancel := context.WithCancel(context.Background())
		emailCancel = workerCancel
		emailDone = make(chan struct{})
		go func() { defer close(emailDone); queue.Run(workerCtx) }()
		invites := service.NewInvitationService(repository.NewInvitationRepository(pool), mail != nil, cfg.AppURL)
		invites.Audit = auditLog
		handlers.Users = handler.NewUsersHandler(service.NewUserAdminService(repository.NewRBACRepository(pool), invites), guard)
		handlers.Invitations = handler.NewInvitationHandler(invites)
		resets := service.NewPasswordResetService(repository.NewPasswordResetRepository(pool), challenges, mail != nil, cfg.AppURL)
		resets.Audit = auditLog
		handlers.PasswordResets = handler.NewPasswordResetHandler(resets)
		handlers.Ledger = handler.NewLedgerHandler(service.NewLedgerService(repository.NewLedgerRepository(pool)), guard)
		db = pool
		log.Println("PostgreSQL connection verified; connection pool ready")
	}
	router := server.New(db, handlers)

	// If -routes flag is provided, print route table and exit
	if *listRoutes {
		fmt.Println("\n==================================================")
		fmt.Println("          PayCore API Registered Endpoints        ")
		fmt.Println("==================================================")
		fmt.Printf("%-8s %-30s %s\n", "METHOD", "PATH", "URL")
		fmt.Println(strings.Repeat("-", 65))
		for _, r := range router.Routes() {
			fmt.Printf("%-8s %-30s http://localhost:%s%s\n", r.Method, r.Path, cfg.Port, r.Path)
		}
		fmt.Println("==================================================")
		fmt.Println()
		return
	}

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("Starting PayCore API server on port %s (%s mode)...", cfg.Port, cfg.Environment)
		log.Println("Active Endpoints:")
		for _, r := range router.Routes() {
			log.Printf("  [%s] http://localhost:%s%s", r.Method, cfg.Port, r.Path)
		}
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			if portInUse(err) {
				log.Fatalf("Server failed to start: port %s is already in use, most likely by an older PayCore API that is still running. Stop it (or restart run.ps1, which stops it) and try again.", cfg.Port)
			}
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Graceful shutdown handling
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	shutdownErr := srv.Shutdown(ctx)
	// Cancel SMTP and leave unfinished claims recoverable by the next process.
	mailCtx, mailCancel := context.WithTimeout(context.Background(), 15*time.Second)
	if emailCancel != nil {
		emailCancel()
		select {
		case <-emailDone:
		case <-mailCtx.Done():
		}
	}
	mailCancel()
	if shutdownErr != nil {
		log.Fatalf("Server forced to shutdown: %v", shutdownErr)
	}

	log.Println("Server exiting")
}

// portInUse reports "address already in use" on every platform (Windows reports
// WSAEADDRINUSE, errno 10048, which is not syscall.EADDRINUSE there).
func portInUse(err error) bool {
	var errno syscall.Errno
	return errors.Is(err, syscall.EADDRINUSE) || (errors.As(err, &errno) && errno == 10048)
}
