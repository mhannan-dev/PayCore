package mfa

import (
	"encoding/base32"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"paycore/api/internal/models"
)

func TestRFC6238SHA1Vectors(t *testing.T) {
	for _, tc := range []struct {
		seconds int64
		code    string
	}{
		{59, "94287082"}, {1111111109, "07081804"}, {1111111111, "14050471"},
		{1234567890, "89005924"}, {2000000000, "69279037"}, {20000000000, "65353130"},
	} {
		if got := otp([]byte("12345678901234567890"), tc.seconds/30, 8); got != tc.code {
			t.Fatalf("RFC vector %d: %s", tc.seconds, got)
		}
	}
}
func TestEnrollmentEncryptionBindingAndReplay(t *testing.T) {
	a, err := New(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	enrollment, encrypted, err := a.Enrollment("owner", "admin@example.test")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(enrollment.QRImage, "data:image/png;base64,") || !strings.HasPrefix(enrollment.URI, "otpauth://totp/") {
		t.Fatal("invalid local enrollment")
	}
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(enrollment.Secret)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encrypted), enrollment.Secret) {
		t.Fatal("secret was not encrypted")
	}
	now := time.Unix(1234567890, 0)
	code := otp(secret, now.Unix()/30, 6)
	counter, err := a.Match("owner", encrypted, code, -1, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.Match("owner", encrypted, code, counter, now); !errors.Is(err, models.ErrMFACode) {
		t.Fatal("replayed time step accepted")
	}
	if _, err = a.Match("other", encrypted, code, -1, now); err == nil {
		t.Fatal("ciphertext could be moved to another user")
	}
	if _, err = a.Match("owner", encrypted, code, -1, now.Add(2*time.Minute)); !errors.Is(err, models.ErrMFACode) {
		t.Fatal("expired code accepted")
	}
}
