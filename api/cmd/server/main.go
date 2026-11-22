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

	"paycore/api/database/migrations"
	"paycore/api/internal/config"
	"paycore/api/internal/database"
	"paycore/api/internal/helpers"
	"paycore/api/internal/http/controllers"
	"paycore/api/internal/http/middleware"
	"paycore/api/internal/mailer"
	"paycore/api/internal/mfa"
	"paycore/api/internal/repositories"
	"paycore/api/internal/server"
	"paycore/api/internal/services"
	"paycore/api/internal/storage"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	listRoutes := flag.Bool("routes", false, "Print all registered API endpoints and exit")
	flag.Parse()

	cfg := config.Load()
	var db server.DatabasePinger
	// Composition root: the only place that creates concrete repositories, services
	// and controllers and wires them together (docs/coding_rules.md rule 25).
	var registeredControllers server.Controllers
	var emailCancel context.CancelFunc
	var emailDone chan struct{}
	var outboxCancel context.CancelFunc
	var outboxDone chan struct{}
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
		challenges := services.NewCaptchaStore()
		authenticator, err := mfa.New(cfg.MFAEncryptionKey)
		if err != nil {
			log.Fatal("MFA_ENCRYPTION_KEY is missing or invalid. Run go run ./cmd/mfa-key from api, or configure a base64-encoded 32-byte key.")
		}
		mfaService := services.NewMFAServiceWithMFAPolicy(repositories.NewMFARepository(pool), authenticator, cfg.MFAEnabled)
		registeredControllers.MFA = controllers.NewMFAController(mfaService)
		authService, err := services.NewAuthServiceWithMFAPolicy(repositories.NewUserRepositoryWithMFAPolicy(pool, cfg.MFAEnabled), challenges, mfaService, cfg.MFAEnabled)
		if err != nil {
			log.Fatal("Authentication initialization failed.")
		}
		auditLog := repositories.NewAuditRepository(pool)
		mfaService.Audit = auditLog
		authService.Audit = auditLog
		authMiddleware := middleware.NewAuth(authService)
		registeredControllers.AuthMiddleware = authMiddleware
		registeredControllers.Security = controllers.NewSecurityController(services.NewSecurityServiceWithMFAPolicy(repositories.NewSecurityRepositoryWithMFAPolicy(pool, cfg.MFAEnabled), authenticator, cfg.MFAEnabled))
		registeredControllers.Auth = controllers.NewAuthController(authService, challenges)
		registeredControllers.Audit = controllers.NewAuditController(auditLog)
		files, err := storage.OpenLocal(cfg.UploadDir)
		if err != nil {
			log.Fatal("Upload storage initialization failed. Check UPLOAD_DIR permissions.")
		}
		defer func() { _ = files.Close() }() // Process exit; nothing left to flush.
		attachments := repositories.NewAttachmentRepository(pool)
		attachmentService := services.NewAttachmentService(attachments, files, helpers.NewWebPConverter(2))
		registeredControllers.Attachments = controllers.NewAttachmentController(attachmentService, services.NewPhotoService(attachments, files))
		registeredControllers.Matching = controllers.NewMatchingController(services.NewMatchingService(repositories.NewMatchingRepository(pool)))
		registeredControllers.Statements = controllers.NewStatementController(services.NewStatementService(attachmentService, repositories.NewLedgerRepository(pool), repositories.NewStatementRepository(pool)))
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
		queue := services.NewEmailWorker(repositories.NewEmailOutboxRepository(pool), mail)
		workerCtx, workerCancel := context.WithCancel(context.Background())
		emailCancel = workerCancel
		emailDone = make(chan struct{})
		go func() { defer close(emailDone); queue.Run(workerCtx) }()
		invites := services.NewInvitationService(repositories.NewInvitationRepository(pool), mail != nil, cfg.AppURL)
		invites.Audit = auditLog
		registeredControllers.Users = controllers.NewUsersController(services.NewUserAdminService(repositories.NewRBACRepository(pool), invites))
		registeredControllers.Invitations = controllers.NewInvitationController(invites)
		resets := services.NewPasswordResetService(repositories.NewPasswordResetRepository(pool), challenges, mail != nil, cfg.AppURL)
		resets.Audit = auditLog
		registeredControllers.PasswordResets = controllers.NewPasswordResetController(resets)
		ledgerService := services.NewLedgerService(repositories.NewLedgerRepository(pool))
		registeredControllers.Ledger = controllers.NewLedgerController(ledgerService)
		settlementService := services.NewSettlementService(repositories.NewSettlementRepository(pool))
		registeredControllers.Settlement = controllers.NewSettlementController(settlementService)
		publisher, err := newOutboxPublisher(cfg.Outbox)
		if err != nil {
			log.Fatalf("Outbox publisher configuration is invalid: %v", err)
		}
		notificationRepo := repositories.NewNotificationRepository(pool)
		notificationService := services.NewNotificationService(notificationRepo)
		notificationService.Audit = auditLog
		registeredControllers.Notifications = controllers.NewNotificationController(notificationService)
		publisher = services.NewNotificationPublisher(notificationRepo, services.SandboxNotificationSink{}, publisher)
		outboxRelay := services.NewOutboxRelay(repositories.NewOutboxRepository(pool), publisher).
			WithBatchSize(cfg.Outbox.BatchSize).
			WithInterval(cfg.Outbox.Interval).
			WithMaxRetries(cfg.Outbox.MaxRetries)
		outboxRelay.Audit = auditLog
		registeredControllers.Outbox = controllers.NewOutboxController(outboxRelay)
		outboxWorkerCtx, outboxWorkerCancel := context.WithCancel(context.Background())
		outboxCancel = outboxWorkerCancel
		outboxDone = make(chan struct{})
		go func() { defer close(outboxDone); outboxRelay.Run(outboxWorkerCtx) }()
		db = pool
		log.Println("PostgreSQL connection verified; connection pool ready")
	}
	router := server.NewContainer(registeredControllers).Router(db)

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
	if outboxCancel != nil {
		outboxCancel()
		select {
		case <-outboxDone:
		case <-time.After(5 * time.Second):
		}
	}
	if shutdownErr != nil {
		log.Fatalf("Server forced to shutdown: %v", shutdownErr)
	}

	log.Println("Server exiting")
}

func newOutboxPublisher(cfg config.OutboxConfig) (services.EventPublisher, error) {
	switch cfg.Publisher {
	case "log":
		return services.NewLogEventPublisher(), nil
	case "webhook":
		return services.NewWebhookEventPublisher(cfg.WebhookURL, cfg.WebhookSecret, 5*time.Second)
	default:
		return nil, fmt.Errorf("unsupported outbox publisher %q", cfg.Publisher)
	}
}

// portInUse reports "address already in use" on every platform (Windows reports
// WSAEADDRINUSE, errno 10048, which is not syscall.EADDRINUSE there).
func portInUse(err error) bool {
	var errno syscall.Errno
	return errors.Is(err, syscall.EADDRINUSE) || (errors.As(err, &errno) && errno == 10048)
}
