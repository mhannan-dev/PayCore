package repositories_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"paycore/api/internal/mailer"
	"paycore/api/internal/models"
	"paycore/api/internal/repositories"
	"paycore/api/internal/services"
)

func digest(s string) []byte { value := sha256.Sum256([]byte(s)); return value[:] }

func emailDraft(u models.EmailRecipient) (models.EmailContent, error) {
	return models.EmailContent{To: u.Email, Subject: "fixture", Text: "sensitive link", HTML: "<p>sensitive link</p>"}, nil
}

func TestDurableInvitationLocalSMTP(t *testing.T) {
	if os.Getenv("SMTP_TEST_HOST") != "127.0.0.1" {
		t.Skip("requires local Mailpit")
	}
	pool := isolatedPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	svc := services.NewInvitationService(repositories.NewInvitationRepository(pool), true, "http://web.test")
	u, status, err := svc.Invite(ctx, models.CreateUserRequest{FirstName: "Durable SMTP", Email: "durable-delivery-check@example.test", Roles: []string{"OPERATOR"}}, "")
	if err != nil || status != "pending" {
		t.Fatalf("persist invitation: %q %v", status, err)
	}
	// Construct delivery only after the invitation transaction has returned.
	worker := services.NewEmailWorker(repositories.NewEmailOutboxRepository(pool), mailer.SMTP{Host: "127.0.0.1", Port: "1025", From: "PayCore <no-reply@paycore.local>", Encryption: mailer.EncryptionNone})
	if err = worker.RunOne(ctx); err != nil {
		t.Fatal(err)
	}
	var body string
	if err = pool.QueryRow(ctx, "SELECT status,text_body FROM email_outbox WHERE user_id=$1", u.ID).Scan(&status, &body); err != nil {
		t.Fatal(err)
	}
	if status != "sent" || body != "" {
		t.Fatal("SMTP acceptance not committed or token body retained")
	}
}

func TestEmailOutboxRecoveryAndFencing(t *testing.T) {
	pool := isolatedPool(t)
	ctx := context.Background()
	invites := repositories.NewInvitationRepository(pool)
	u, err := invites.CreateInvitedUser(ctx, "Durable", nil, "durable@example.test", []string{"OPERATOR"}, digest("durable"), time.Now().Add(time.Hour), "", emailDraft)
	if err != nil {
		t.Fatal(err)
	}
	first := repositories.NewEmailOutboxRepository(pool)
	job, err := first.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if job.Content.To != u.Email || job.Content.Text != "sensitive link" || job.Attempts != 1 {
		t.Fatalf("wrong persisted job: %+v", job)
	}
	if _, err = first.Claim(ctx); !errors.Is(err, models.ErrNoEmailJob) {
		t.Fatalf("leased job claimed twice: %v", err)
	}
	if _, err = pool.Exec(ctx, "UPDATE email_outbox SET lease_until=NOW()-INTERVAL '1 second' WHERE id=$1", job.ID); err != nil {
		t.Fatal(err)
	}
	// Reconstruct the repository as a restarted API would, with no process memory.
	restarted := repositories.NewEmailOutboxRepository(pool)
	recovered, err := restarted.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.ID != job.ID || recovered.LeaseID == job.LeaseID || recovered.Attempts != 2 {
		t.Fatal("expired lease was not reclaimed")
	}
	if err = first.Complete(ctx, job, models.EmailOutcome{Status: models.EmailDelivered}); !errors.Is(err, models.ErrEmailLeaseLost) {
		t.Fatalf("old worker overwrote new lease: %v", err)
	}
	if err = restarted.Complete(ctx, recovered, models.EmailOutcome{Status: models.EmailDelivered}); err != nil {
		t.Fatal(err)
	}
	var status, body, html string
	var attempts int
	if err = pool.QueryRow(ctx, `SELECT status,text_body,html_body FROM email_outbox WHERE id=$1`, job.ID).Scan(&status, &body, &html); err != nil {
		t.Fatal(err)
	}
	if status != "sent" || body != "" || html != "" {
		t.Fatal("terminal job retained one-time links")
	}
	if err = pool.QueryRow(ctx, "SELECT email_status,email_attempts FROM user_invitations WHERE user_id=$1", u.ID).Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "sent" || attempts != 2 {
		t.Fatal("invitation delivery record not committed with outcome")
	}
}

func TestEmailOutboxConcurrentClaims(t *testing.T) {
	pool := isolatedPool(t)
	ctx := context.Background()
	invites := repositories.NewInvitationRepository(pool)
	for i := 0; i < 8; i++ {
		label := fmt.Sprintf("parallel-%d", i)
		if _, err := invites.CreateInvitedUser(ctx, label, nil, label+"@example.test", []string{"OPERATOR"}, digest(label), time.Now().Add(time.Hour), "", emailDraft); err != nil {
			t.Fatal(err)
		}
	}
	ids := make(chan string, 8)
	failures := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			job, err := repositories.NewEmailOutboxRepository(pool).Claim(ctx)
			if err != nil {
				failures <- err
				return
			}
			ids <- job.ID
		}()
	}
	wg.Wait()
	close(ids)
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	seen := map[string]bool{}
	for id := range ids {
		if seen[id] {
			t.Fatal("job leased concurrently")
		}
		seen[id] = true
	}
	if len(seen) != 8 {
		t.Fatalf("got %d distinct claims", len(seen))
	}
}

func TestEmailOutboxRetryAndStaleTokenCleanup(t *testing.T) {
	pool := isolatedPool(t)
	ctx := context.Background()
	invites := repositories.NewInvitationRepository(pool)
	jobs := repositories.NewEmailOutboxRepository(pool)
	u, err := invites.CreateInvitedUser(ctx, "Retry", nil, "retry@example.test", []string{"OPERATOR"}, digest("retry"), time.Now().Add(time.Hour), "", emailDraft)
	if err != nil {
		t.Fatal(err)
	}
	job, err := jobs.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = jobs.Complete(ctx, job, models.EmailOutcome{Status: models.EmailQueued, ErrorCode: "smtp_temporary_failure", RetryAfter: time.Hour}); err != nil {
		t.Fatal(err)
	}
	if _, err = jobs.Claim(ctx); !errors.Is(err, models.ErrNoEmailJob) {
		t.Fatalf("retry ignored schedule: %v", err)
	}
	if _, err = invites.ReissueInvitation(ctx, u.ID, digest("replacement"), time.Now().Add(time.Hour), "", emailDraft); err != nil {
		t.Fatal(err)
	}
	replacement, err := jobs.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if replacement.ID == job.ID {
		t.Fatal("superseded invitation was sent")
	}
	var status, body string
	if err = pool.QueryRow(ctx, "SELECT status,text_body FROM email_outbox WHERE id=$1", job.ID).Scan(&status, &body); err != nil {
		t.Fatal(err)
	}
	if status != "canceled" || body != "" {
		t.Fatal("superseded job retained its usable link")
	}
	if err = jobs.Complete(ctx, replacement, models.EmailOutcome{Status: models.EmailDeliveryFailed, ErrorCode: "smtp_permanent_failure"}); err != nil {
		t.Fatal(err)
	}
}

func TestOutboxFailureRollsBackUserAndToken(t *testing.T) {
	pool := isolatedPool(t)
	ctx := context.Background()
	invites := repositories.NewInvitationRepository(pool)
	// Force failure at the actual email INSERT, after user/invitation INSERTs.
	if _, err := pool.Exec(ctx, `CREATE FUNCTION reject_outbox() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture outbox unavailable'; END $$;
	 CREATE TRIGGER reject_outbox BEFORE INSERT ON email_outbox FOR EACH ROW EXECUTE FUNCTION reject_outbox()`); err != nil {
		t.Fatal(err)
	}
	if _, err := invites.CreateInvitedUser(ctx, "Rollback", nil, "rollback@example.test", []string{"OPERATOR"}, digest("rollback"), time.Now().Add(time.Hour), "", emailDraft); err == nil {
		t.Fatal("outbox failure reported success")
	}
	var users, tokens, jobs int
	if err := pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM users),(SELECT count(*) FROM user_invitations),(SELECT count(*) FROM email_outbox)").Scan(&users, &tokens, &jobs); err != nil {
		t.Fatal(err)
	}
	if users != 0 || tokens != 0 || jobs != 0 {
		t.Fatalf("partial transaction: users=%d tokens=%d jobs=%d", users, tokens, jobs)
	}
}

func TestResetEmailFailurePreservesPreviousToken(t *testing.T) {
	pool := isolatedPool(t)
	ctx := context.Background()
	users := repositories.NewUserRepository(pool)
	u, err := users.Create(ctx, "Reset", nil, "reset-durable@example.test", "fixture hash", []string{"OPERATOR"})
	if err != nil {
		t.Fatal(err)
	}
	resets := repositories.NewPasswordResetRepository(pool)
	if _, err = resets.IssueResetToken(ctx, u.Email, digest("old"), time.Now().Add(time.Hour), 0, emailDraft); err != nil {
		t.Fatal(err)
	}
	broken := func(models.EmailRecipient) (models.EmailContent, error) {
		return models.EmailContent{}, errors.New("template failed")
	}
	if _, err = resets.IssueResetToken(ctx, u.Email, digest("new"), time.Now().Add(time.Hour), 0, broken); err == nil {
		t.Fatal("failed email intent committed reset")
	}
	if _, err = resets.FindResetToken(ctx, digest("old")); err != nil {
		t.Fatalf("rollback lost previous reset: %v", err)
	}
	jobs := repositories.NewEmailOutboxRepository(pool)
	job, err := jobs.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != models.EmailPasswordReset {
		t.Fatal("wrong reset job kind")
	}
	if _, err = resets.ResetPassword(ctx, digest("old"), "new fixture hash"); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "UPDATE email_outbox SET lease_until=NOW()-INTERVAL '1 second' WHERE id=$1", job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = jobs.Claim(ctx); !errors.Is(err, models.ErrNoEmailJob) {
		t.Fatal("used reset token was delivered")
	}
}

func TestEmailOutboxRejectsExpiredInactiveAndExhausted(t *testing.T) {
	for _, mode := range []string{"expired", "inactive", "exhausted"} {
		t.Run(mode, func(t *testing.T) {
			pool := isolatedPool(t)
			ctx := context.Background()
			u, err := repositories.NewInvitationRepository(pool).CreateInvitedUser(ctx, "Stop", nil, "stop@example.test", []string{"OPERATOR"}, digest("stop"), time.Now().Add(time.Hour), "", emailDraft)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "expired":
				_, err = pool.Exec(ctx, "UPDATE email_outbox SET expires_at=NOW()-INTERVAL '1 second'")
			case "inactive":
				_, err = pool.Exec(ctx, "UPDATE users SET is_active=false WHERE id=$1", u.ID)
			case "exhausted":
				_, err = pool.Exec(ctx, "UPDATE email_outbox SET attempts=5")
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = repositories.NewEmailOutboxRepository(pool).Claim(ctx); !errors.Is(err, models.ErrNoEmailJob) {
				t.Fatalf("ineligible email claimed: %v", err)
			}
			var status, body string
			if err = pool.QueryRow(ctx, "SELECT status,text_body FROM email_outbox WHERE user_id=$1", u.ID).Scan(&status, &body); err != nil {
				t.Fatal(err)
			}
			want := "canceled"
			if mode == "exhausted" {
				want = "failed"
			}
			if status != want || body != "" {
				t.Fatalf("ineligible job was not scrubbed: %q", status)
			}
		})
	}
}
