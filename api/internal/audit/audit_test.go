package audit

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

type deadlineRecorder struct{ checked bool }

func (r *deadlineRecorder) Record(ctx context.Context, _ Event) error {
	deadline, ok := ctx.Deadline()
	r.checked = ok && time.Until(deadline) > 0 && time.Until(deadline) <= 2*time.Second
	return nil
}
func TestDetachedAuditStillHasDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := &deadlineRecorder{}
	Record(ctx, r, Event{Action: LoginSucceeded})
	if !r.checked {
		t.Fatal("detached audit must have its own timeout")
	}
}
func TestUserAgentTruncationPreservesUTF8(t *testing.T) {
	e := Complete(WithMeta(context.Background(), Meta{UserAgent: strings.Repeat("বাংলা", 100)}), Event{})
	if e.UserAgent == nil || !utf8.ValidString(*e.UserAgent) || utf8.RuneCountInString(*e.UserAgent) > 300 {
		t.Fatal("invalid truncated user agent")
	}
}
