// Package mfa provides local TOTP and authenticated encryption primitives.
package mfa

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // HMAC-SHA1 is required for interoperable RFC 6238 authenticator codes.
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	qrcode "github.com/skip2/go-qrcode"
	"paycore/api/internal/models"
)

type Authenticator struct{ cipher cipher.AEAD }

func New(key string) (*Authenticator, error) {
	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil || len(decoded) != 32 {
		return nil, errors.New("MFA_ENCRYPTION_KEY must be base64-encoded 32 random bytes")
	}
	block, err := aes.NewCipher(decoded)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Authenticator{cipher: aead}, nil
}
func (a *Authenticator) Enrollment(userID, email string) (models.MFAEnrollment, []byte, error) {
	secret := make([]byte, 20)
	if _, err := rand.Read(secret); err != nil {
		return models.MFAEnrollment{}, nil, err
	}
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)
	uri := url.URL{Scheme: "otpauth", Host: "totp", Path: "/PayCore:" + email}
	query := url.Values{"secret": {encoded}, "issuer": {"PayCore"}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}
	uri.RawQuery = query.Encode()
	png, err := qrcode.Encode(uri.String(), qrcode.Medium, 256)
	if err != nil {
		return models.MFAEnrollment{}, nil, err
	}
	nonce := make([]byte, a.cipher.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return models.MFAEnrollment{}, nil, err
	}
	encrypted := a.cipher.Seal(nonce, nonce, secret, []byte(userID))
	return models.MFAEnrollment{Secret: encoded, URI: uri.String(), QRImage: "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)}, encrypted, nil
}
func (a *Authenticator) Match(userID string, encrypted []byte, code string, last int64, now time.Time) (int64, error) {
	if len(code) != 6 {
		return -1, models.ErrMFACode
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return -1, models.ErrMFACode
		}
	}
	if len(encrypted) < a.cipher.NonceSize() {
		return -1, errors.New("invalid MFA encrypted secret")
	}
	n := a.cipher.NonceSize()
	secret, err := a.cipher.Open(nil, encrypted[:n], encrypted[n:], []byte(userID))
	if err != nil {
		return -1, errors.New("MFA secret could not be decrypted")
	}
	for counter := now.Unix()/30 - 1; counter <= now.Unix()/30+1; counter++ {
		if counter > last && hmac.Equal([]byte(otp(secret, counter, 6)), []byte(code)) {
			return counter, nil
		}
	}
	return -1, models.ErrMFACode
}
func otp(secret []byte, counter int64, digits int) string {
	if counter < 0 {
		return ""
	}
	var value [8]byte
	binary.BigEndian.PutUint64(value[:], uint64(counter))
	// HMAC-SHA1 is the interoperable RFC 6238 TOTP algorithm, not password hashing.
	mac := hmac.New(sha1.New, secret)
	_, _ = mac.Write(value[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 15
	number := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	modulus := uint32(1)
	for i := 0; i < digits; i++ {
		modulus *= 10
	}
	return fmt.Sprintf("%0"+strconv.Itoa(digits)+"d", number%modulus)
}
