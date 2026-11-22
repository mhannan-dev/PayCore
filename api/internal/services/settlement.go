package services

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"paycore/api/internal/models"
)

type SettlementStore interface {
	GetByID(context.Context, string) (models.Settlement, error)
	List(context.Context, models.SettlementQuery) ([]models.Settlement, int, error)
	Execute(context.Context, string, string, string, func(context.Context, models.SettlementWriter) (int, []byte, error)) (models.IdempotentResult, error)
}
type SettlementService struct{ store SettlementStore }

func NewSettlementService(store SettlementStore) *SettlementService {
	return &SettlementService{store: store}
}

func newSettlementID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate payout ID: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

// holdsSystemRole reports whether a is active and holds the given system role (migration 024).
func holdsSystemRole(a models.Account, role models.AccountSystemRole) bool {
	return a.IsActive && a.SystemRole != nil && *a.SystemRole == role
}

// findAccount returns the first account matching match, or missing if none does.
func findAccount(accounts []models.Account, missing error, match func(models.Account) bool) (models.Account, error) {
	for _, a := range accounts {
		if match(a) {
			return a, nil
		}
	}
	return models.Account{}, missing
}

// postTransfer posts a two-leg journal moving amount from the debit account to the credit account.
func postTransfer(ctx context.Context, w models.SettlementWriter, actor models.UserDTO, currency models.Currency,
	reference, description, debit, credit string, amount models.Amount) (models.Journal, error) {
	return w.PostJournal(ctx, models.NewJournal{
		ReferenceID: reference, Description: description, Currency: currency,
		Legs: []models.Leg{
			{AccountID: debit, Direction: models.Debit, Amount: amount},
			{AccountID: credit, Direction: models.Credit, Amount: amount},
		},
		CreatedBy: actor.ID, CreatedByEmail: actor.Email,
	})
}

func (s *SettlementService) Reserve(ctx context.Context, actor models.UserDTO, key string, req models.CreatePayoutRequest) (models.IdempotentResult, error) {
	if err := req.Normalize(); err != nil {
		return models.IdempotentResult{}, err
	}
	amount, err := models.ParseAmount(req.Amount) // Already validated by Normalize.
	if err != nil {
		return models.IdempotentResult{}, err
	}
	hash, err := requestHash("POST", "/api/v1/settlements", req)
	if err != nil {
		return models.IdempotentResult{}, err
	}
	return s.store.Execute(ctx, actor.ID, key, hash, func(ctx context.Context, w models.SettlementWriter) (int, []byte, error) {
		accounts, err := w.Accounts(ctx, req.Currency)
		if err != nil {
			return 0, nil, err
		}
		source, err := findAccount(accounts, models.ErrPayoutSource, func(a models.Account) bool { return a.ID == req.SourceAccountID })
		// A source is an ordinary customer wallet: never a system account.
		if err != nil || !source.IsActive || source.Type != models.AccountLiability || source.SystemRole != nil {
			return 0, nil, models.ErrPayoutSource
		}
		clearing, err := findAccount(accounts, models.ErrPayoutClearing, func(a models.Account) bool { return holdsSystemRole(a, models.RolePayoutClearing) })
		if err != nil {
			return 0, nil, err
		}
		id, err := newSettlementID()
		if err != nil {
			return 0, nil, err
		}
		reference := "STL-" + id
		journal, err := postTransfer(ctx, w, actor, req.Currency, reference, "Payout reservation for "+req.BeneficiaryName,
			source.ID, clearing.ID, amount)
		if err != nil {
			return 0, nil, err
		}
		now := time.Now().UTC()
		payout := models.Settlement{ID: id, PayoutReference: reference, SourceAccountID: source.ID, SourceAccountName: source.Name,
			ClearingAccountID: clearing.ID, ClearingAccountName: clearing.Name, Amount: amount, Currency: req.Currency,
			Status: models.SettlementReserved, ProviderMode: models.ProviderSandbox, ProviderReference: "SANDBOX-" + id,
			BeneficiaryName: req.BeneficiaryName, BeneficiaryBank: req.BeneficiaryBank, BeneficiaryAccount: req.BeneficiaryAccount,
			ReserveJournalID: journal.ID, CreatedBy: &actor.ID, CreatedAt: now, UpdatedAt: now}
		if err := w.CreateSettlement(ctx, payout); err != nil {
			return 0, nil, err
		}
		body, err := json.Marshal(payout)
		return http.StatusCreated, body, err
	})
}
func (s *SettlementService) Capture(ctx context.Context, actor models.UserDTO, key, id string) (models.IdempotentResult, error) {
	return s.transition(ctx, actor, key, id, models.SettlementCaptured)
}
func (s *SettlementService) Release(ctx context.Context, actor models.UserDTO, key, id string) (models.IdempotentResult, error) {
	return s.transition(ctx, actor, key, id, models.SettlementReleased)
}

// transition moves a RESERVED sandbox payout out of clearing: to the operating
// bank on capture, or back to the source wallet on release.
func (s *SettlementService) transition(ctx context.Context, actor models.UserDTO, key, id string, target models.SettlementStatus) (models.IdempotentResult, error) {
	if !models.ValidUUID(id) {
		return models.IdempotentResult{}, models.InvalidRequestError{Code: "invalid_id", Reason: "Settlement ID must be a UUID."}
	}
	hash, err := requestHash("POST", "/api/v1/settlements/"+id, target)
	if err != nil {
		return models.IdempotentResult{}, err
	}
	return s.store.Execute(ctx, actor.ID, key, hash, func(ctx context.Context, w models.SettlementWriter) (int, []byte, error) {
		payout, err := w.GetSettlement(ctx, id)
		if err != nil {
			return 0, nil, err
		}
		if payout.Status != models.SettlementReserved {
			return 0, nil, models.ErrPayoutState
		}
		if payout.ProviderMode != models.ProviderSandbox {
			return 0, nil, models.ErrPayoutProvider
		}
		destination := payout.SourceAccountID
		description := "Payout release for "
		if target == models.SettlementCaptured {
			accounts, err := w.Accounts(ctx, payout.Currency)
			if err != nil {
				return 0, nil, err
			}
			bank, err := findAccount(accounts, models.ErrPayoutBank, func(a models.Account) bool { return holdsSystemRole(a, models.RoleOperatingBank) })
			if err != nil {
				return 0, nil, err
			}
			destination = bank.ID
			payout.DestinationAccountID = &destination
			description = "Payout capture for "
		}
		journal, err := postTransfer(ctx, w, actor, payout.Currency, payout.PayoutReference+"-"+string(target),
			description+payout.BeneficiaryName, payout.ClearingAccountID, destination, payout.Amount)
		if err != nil {
			return 0, nil, err
		}
		payout.Status = target
		payout.UpdatedAt = time.Now().UTC()
		if target == models.SettlementCaptured {
			payout.CaptureJournalID = &journal.ID
		} else {
			payout.ReleaseJournalID = &journal.ID
		}
		if err := w.UpdateSettlement(ctx, payout); err != nil {
			return 0, nil, err
		}
		body, err := json.Marshal(payout)
		return http.StatusOK, body, err
	})
}
func (s *SettlementService) List(ctx context.Context, q models.SettlementQuery) ([]models.Settlement, int, error) {
	if q.Currency != "" && !q.Currency.Valid() {
		return nil, 0, models.InvalidRequestError{Code: "invalid_currency", Reason: "Currency must be JPY, USD or EUR."}
	}
	return s.store.List(ctx, q)
}
func (s *SettlementService) Get(ctx context.Context, id string) (models.Settlement, error) {
	if !models.ValidUUID(id) {
		return models.Settlement{}, models.InvalidRequestError{Code: "invalid_id", Reason: "Settlement ID must be a UUID."}
	}
	return s.store.GetByID(ctx, id)
}
