package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"paycore/api/internal/config"
	"paycore/api/internal/database"
	"paycore/api/internal/handler"
	"paycore/api/internal/mailer"
	"paycore/api/internal/repository"
	"paycore/api/internal/server"
	"paycore/api/internal/service"
	"paycore/api/internal/storage"
	"paycore/api/migrations"
)

func main() {
	listRoutes := flag.Bool("routes", false, "Print all registered API endpoints and exit")
	flag.Parse()

	cfg := config.Load()
	var db server.DatabasePinger
	var authHandler *handler.AuthHandler
	var attachmentHandler *handler.AttachmentHandler
	var rbacHandler *handler.RBACHandler
	var invites *service.InvitationService
	var resets *service.PasswordResetService
	var resetHandler *handler.PasswordResetHandler
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
		authService, err := service.NewAuthService(repository.NewUserRepository(pool), challenges)
		if err != nil {
			log.Fatal("Authentication initialization failed.")
		}
		authHandler = handler.NewAuthHandler(authService, challenges)
		files, err := storage.OpenLocal(cfg.UploadDir)
		if err != nil {
			log.Fatal("Upload storage initialization failed. Check UPLOAD_DIR permissions.")
		}
		defer files.Close()
		attachmentHandler = handler.NewAttachmentHandler(authService, repository.NewAttachmentRepository(pool), files)
		var mail mailer.Sender
		switch {
		case cfg.SMTPHost != "":
			smtpSender := mailer.SMTP{Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUsername, Password: cfg.SMTPPassword, From: cfg.SMTPFrom, Encryption: cfg.SMTPEncryption}
			if err := smtpSender.Validate(); err != nil {
				log.Fatalf("Email configuration (%s) is invalid: %v", cfg.SMTPSource, err)
			}
			mail = smtpSender
			auth := "no auth"
			if cfg.SMTPUsername != "" {
				auth = "auth as " + cfg.SMTPUsername
			}
			log.Printf("Email: SMTP %s:%s (%s, %s, from %s; from %s variables)", cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPEncryption, auth, cfg.SMTPFrom, cfg.SMTPSource)
		case cfg.Environment == "development":
			mail = mailer.Log{}
			log.Println("Email: SMTP_HOST is not set; invitation emails are written to this log (development only)")
		default:
			log.Println("Email: SMTP_HOST is not set; invitations are created but not emailed")
		}
		invites = service.NewInvitationService(repository.NewInvitationRepository(pool), mail, cfg.AppURL)
		rbacHandler = handler.NewRBACHandler(authService, repository.NewRBACRepository(pool), invites)
		resets = service.NewPasswordResetService(repository.NewPasswordResetRepository(pool), challenges, mail, cfg.AppURL)
		resetHandler = handler.NewPasswordResetHandler(resets)
		db = pool
		log.Println("PostgreSQL connection verified; connection pool ready")
	}
	router := server.SetupRouterWithAttachments(db, authHandler, attachmentHandler, rbacHandler, resetHandler)

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
	// Let in-flight emails finish; invitations cut short stay "pending" and can be re-sent.
	mailCtx, mailCancel := context.WithTimeout(context.Background(), 15*time.Second)
	if invites != nil {
		invites.Shutdown(mailCtx)
	}
	if resets != nil {
		resets.Shutdown(mailCtx)
	}
	mailCancel()
	if shutdownErr != nil {
		log.Fatalf("Server forced to shutdown: %v", shutdownErr)
	}

	log.Println("Server exiting")
}
