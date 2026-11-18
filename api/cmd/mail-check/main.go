// Command mail-check verifies the API's outgoing email settings.
//
//	go run ./cmd/mail-check                  # connect, STARTTLS/TLS and log in; sends nothing
//	go run ./cmd/mail-check -to you@x.com    # also send a test email
//
// It reads the same SMTP_* / MAIL_* variables (and api/.env) as the server, and never
// prints the password.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"paycore/api/internal/config"
	"paycore/api/internal/mailer"
)

func main() {
	to := flag.String("to", "", "send a test email to this address (optional)")
	flag.Parse()

	cfg := config.Load()
	if cfg.SMTPHost == "" {
		fmt.Println("No mail server configured: set SMTP_HOST (or MAIL_HOST) in the environment or api/.env.")
		os.Exit(2)
	}
	sender := mailer.SMTP{Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUsername, Password: cfg.SMTPPassword, From: cfg.SMTPFrom, Encryption: cfg.SMTPEncryption}
	password := "(not set)"
	if cfg.SMTPPassword != "" {
		password = "(set)"
	}
	fmt.Printf("Settings from %s variables\n  server:     %s:%s\n  encryption: %s\n  username:   %s\n  password:   %s\n  from:       %s\n  app url:    %s\n",
		cfg.SMTPSource, cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPEncryption, cfg.SMTPUsername, password, cfg.SMTPFrom, cfg.AppURL)

	if err := sender.Validate(); err != nil {
		fmt.Println("\nFAIL configuration:", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	start := time.Now()
	if err := sender.Check(ctx); err != nil {
		fmt.Println("\nFAIL connect/encrypt/login:", err)
		os.Exit(1)
	}
	fmt.Printf("\nOK   connected, encrypted and logged in (%s)\n", time.Since(start).Round(time.Millisecond))

	if *to == "" {
		fmt.Println("     No email sent. Add -to you@example.com to send a test message.")
		return
	}
	start = time.Now()
	err := sender.Send(ctx, mailer.Message{
		To:      *to,
		Subject: "PayCore email test",
		Text:    "This is a test email from PayCore's mail-check command. If you can read it, invitation emails will be delivered.\n",
		HTML:    "<p>This is a test email from PayCore's <code>mail-check</code> command.</p><p>If you can read it, invitation emails will be delivered.</p>",
	})
	if err != nil {
		fmt.Println("FAIL send:", err)
		os.Exit(1)
	}
	fmt.Printf("OK   test email accepted by the server for %s (%s). Check that inbox (and spam).\n", *to, time.Since(start).Round(time.Millisecond))
}
