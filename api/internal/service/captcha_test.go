package service

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCaptchaSingleUseAndExpiry(t *testing.T) {
	now := time.Now()
	s := NewCaptchaStore()
	s.now = func() time.Time { return now }
	insert := func(id string) {
		s.entries[id] = challenge{digest: sha256.Sum256([]byte(id + ":12")), expires: now.Add(time.Minute)}
	}
	insert("correct")
	if !s.Verify("correct", "12") || s.Verify("correct", "12") {
		t.Fatal("challenge must succeed only once")
	}
	insert("wrong")
	if s.Verify("wrong", "0") || s.Verify("wrong", "12") {
		t.Fatal("wrong answer must consume challenge")
	}
	insert("expired")
	now = now.Add(time.Minute)
	if s.Verify("expired", "12") {
		t.Fatal("expired challenge accepted")
	}
}
func TestConcurrentCaptchaConsumption(t *testing.T) {
	s := NewCaptchaStore()
	s.entries["race"] = challenge{digest: sha256.Sum256([]byte("race:12")), expires: time.Now().Add(time.Minute)}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.Verify("race", "12") {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatal("concurrent challenge replay succeeded")
	}
}
func TestCaptchaIssue(t *testing.T) {
	s := NewCaptchaStore()
	c, err := s.Issue()
	if err != nil || len(c.ID) != 64 || !strings.HasPrefix(c.Image, "data:image/svg+xml;base64,") || c.ExpiresIn != 120 {
		t.Fatal("invalid image challenge")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(c.Image, "data:image/svg+xml;base64,"))
	if err != nil || !strings.HasPrefix(string(raw), "<svg") || !strings.Contains(string(raw), ">?</text>") {
		t.Fatal("challenge image is not a math question SVG")
	}
}

// Issue must store the digest of the real answer, and Verify must accept it
// (also with a leading zero) exactly once.
func TestMathCaptchaAnswer(t *testing.T) {
	for i := 0; i < 200; i++ {
		question, answer, err := mathQuestion()
		if err != nil {
			t.Fatal(err)
		}
		var a, b int
		var op string
		if _, err := fmt.Sscanf(question, "%d %s %d", &a, &op, &b); err != nil {
			t.Fatalf("unparseable question %q", question)
		}
		want := map[string]int{"+": a + b, "-": a - b, "×": a * b}[op]
		if answer != want || answer < 0 || answer > 999 {
			t.Fatalf("%q: answer %d, want %d", question, answer, want)
		}
	}
	s := NewCaptchaStore()
	if _, err := s.Issue(); err != nil {
		t.Fatal(err)
	}
	for id, entry := range s.entries {
		answer := -1
		for n := 0; n <= 999; n++ {
			if answerDigest(id, strconv.Itoa(n)) == entry.digest {
				answer = n
				break
			}
		}
		if answer < 0 {
			t.Fatal("stored digest matches no numeric answer")
		}
		if !s.Verify(id, " 0"+strconv.Itoa(answer)+" ") || s.Verify(id, strconv.Itoa(answer)) {
			t.Fatal("correct answer must succeed exactly once")
		}
	}
	s.entries["neg"] = challenge{digest: answerDigest("neg", "5"), expires: time.Now().Add(time.Minute)}
	if s.Verify("neg", "-5") {
		t.Fatal("negative answer accepted")
	}
}
