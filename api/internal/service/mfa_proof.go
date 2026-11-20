package service

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"paycore/api/internal/model"
)

func verifyMFAProof(auth Authenticator, state model.MFARecord, code string, now time.Time) (model.MFAApproval, bool, error) {
	code = strings.ToLower(strings.TrimSpace(code))
	approval := model.MFAApproval{Counter: state.LastCounter}
	if len(state.Secret) > 0 && len(code) == 32 {
		digest := sha256.Sum256([]byte(code))
		for _, hash := range state.RecoveryHashes {
			if hmac.Equal(digest[:], hash) {
				approval.RecoveryHash = digest[:]
				return approval, true, nil
			}
		}
		return model.MFAApproval{}, false, model.ErrMFACode
	}
	secret := state.Secret
	if len(secret) == 0 {
		secret = state.PendingSecret
	}
	if len(secret) == 0 {
		return model.MFAApproval{}, false, model.ErrMFARequired
	}
	counter, err := auth.Match(state.UserID, secret, code, state.LastCounter, now)
	if err != nil {
		return model.MFAApproval{}, false, err
	}
	approval.Counter = counter
	return approval, false, nil
}
func newRecoveryCodes() ([]string, [][]byte, error) {
	codes := make([]string, 0, 8)
	hashes := make([][]byte, 0, 8)
	for i := 0; i < 8; i++ {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return nil, nil, err
		}
		code := hex.EncodeToString(random[:])
		codes = append(codes, code)
		hash := sha256.Sum256([]byte(code))
		hashes = append(hashes, hash[:])
	}
	return codes, hashes, nil
}
