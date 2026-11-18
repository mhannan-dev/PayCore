package mailer

import (
	"context"
	"errors"
	"net/mail"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDevelopmentLogDoesNotReportDelivery(t *testing.T) {
	err := (Log{}).Send(context.Background(), Message{To: "preview@example.test", Subject: "Preview", Text: "No delivery occurred."})
	if !errors.Is(err, ErrNotDelivered) {
		t.Fatal("development logging must not report SMTP delivery")
	}
}

func TestBuildMultipart(t *testing.T) {
	from, _ := mail.ParseAddress("PayCore <no-reply@paycore.local>")
	body, err := build(from, "mina@example.test", Message{Subject: "Set your PayCore password", Text: "Hello\nlink", HTML: "<p>Hello</p>"})
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, want := range []string{"To: mina@example.test\r\n", "Subject: Set your PayCore password\r\n", "multipart/alternative", "text/plain; charset=utf-8", "text/html; charset=utf-8", "Hello\r\nlink"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in\n%s", want, s)
		}
	}
}

func TestSMTPRejectsHeaderInjection(t *testing.T) {
	err := SMTP{Host: "127.0.0.1", Port: "1", From: "a@b.test"}.Send(context.Background(), Message{To: "x@example.test", Subject: "hi\r\nBcc: evil@example.test"})
	if err == nil || !strings.Contains(err.Error(), "single-line") {
		t.Fatalf("got %v", err)
	}
}

// Sends a real message when SMTP_TEST_HOST is set (e.g. Mailpit: SMTP_TEST_HOST=127.0.0.1 SMTP_TEST_PORT=1025).
func TestSMTPDelivery(t *testing.T) {
	host := os.Getenv("SMTP_TEST_HOST")
	if host == "" {
		t.Skip("set SMTP_TEST_HOST to send a real email")
	}
	port := os.Getenv("SMTP_TEST_PORT")
	if port == "" {
		port = "1025"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := SMTP{Host: host, Port: port, From: "PayCore <no-reply@paycore.local>"}.Send(ctx, Message{
		To: "smtp-test@example.test", Subject: "PayCore SMTP test", Text: "plain body", HTML: "<p>html body</p>",
	})
	if err != nil {
		t.Fatal(err)
	}
}
