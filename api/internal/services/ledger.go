package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"paycore/api/internal/models"
	"paycore/api/internal/repositories"
)

// ErrCannotReverseReversal: a reversal journal cannot itself be reversed.
var ErrCannotReverseReversal = errors.New("a reversal cannot be reversed")

// LedgerStore is the ledger storage the service needs (implemented by
// repositories.LedgerRepository).
type LedgerStore interface {
	CreateAccount(ctx context.Context, req models.CreateAccountRequest, createdBy string) (models.Account, error)
	GetAccount(ctx context.Context, id string) (models.Account, error)
	ListAccounts(ctx context.Context, q models.AccountQuery) ([]models.Account, int, error)
	ListAccountPostings(ctx context.Context, accountID string, limit, offset int) ([]models.AccountPosting, int, error)
	GetJournal(ctx context.Context, id string) (models.Journal, error)
	ListJournals(ctx context.Context, q models.JournalQuery) ([]models.Journal, int, error)
	ZeroSum(ctx context.Context, currency models.Currency) (models.ZeroSumReport, error)
	Idempotent(ctx context.Context, clientID, key, requestHash string,
		op func(ctx context.Context, w models.LedgerWriter) (int, []byte, error)) (models.IdempotentResult, error)
}

// LedgerService implements the double-entry ledger: accounts, transfers,
// reversals and the zero-sum audit. Money moves only through Transfer and
// Reverse, which are idempotent per (user, Idempotency-Key).
type LedgerService struct{ store LedgerStore }

func NewLedgerService(store LedgerStore) *LedgerService { return &LedgerService{store: store} }

func (s *LedgerService) CreateAccount(ctx context.Context, actor models.UserDTO, req models.CreateAccountRequest) (models.Account, error) {
	if err := req.Normalize(); err != nil {
		return models.Account{}, err
	}
	return s.store.CreateAccount(ctx, req, actor.ID)
}

func (s *LedgerService) Account(ctx context.Context, id string) (models.Account, error) {
	return s.store.GetAccount(ctx, id)
}

func (s *LedgerService) Accounts(ctx context.Context, q models.AccountQuery) ([]models.Account, int, error) {
	return s.store.ListAccounts(ctx, q)
}

func (s *LedgerService) AccountPostings(ctx context.Context, accountID string, limit, offset int) ([]models.AccountPosting, int, error) {
	return s.store.ListAccountPostings(ctx, accountID, limit, offset)
}

func (s *LedgerService) Journal(ctx context.Context, id string) (models.Journal, error) {
	return s.store.GetJournal(ctx, id)
}

func (s *LedgerService) Journals(ctx context.Context, q models.JournalQuery) ([]models.Journal, int, error) {
	return s.store.ListJournals(ctx, q)
}

// ZeroSum audits that total debits equal total credits for a currency.
func (s *LedgerService) ZeroSum(ctx context.Context, currency models.Currency) (models.ZeroSumReport, error) {
	if !currency.Valid() {
		return models.ZeroSumReport{}, models.InvalidRequestError{Code: "invalid_currency", Reason: "currency must be JPY, USD or EUR."}
	}
	return s.store.ZeroSum(ctx, currency)
}

// Transfer posts a balanced journal. The result is the HTTP status and JSON body
// to send; a retry with the same key and request replays them exactly.
func (s *LedgerService) Transfer(ctx context.Context, actor models.UserDTO, key string, req models.TransferRequest) (models.IdempotentResult, error) {
	if err := req.Normalize(); err != nil {
		return models.IdempotentResult{}, err
	}
	hash, err := requestHash(http.MethodPost, "/api/v1/transfers", req)
	if err != nil {
		return models.IdempotentResult{}, err
	}
	return s.store.Idempotent(ctx, actor.ID, key, hash, func(ctx context.Context, w models.LedgerWriter) (int, []byte, error) {
		j, err := w.PostJournal(ctx, models.NewJournal{
			ReferenceID: req.ReferenceID, Description: req.Description, Currency: req.Currency, Legs: req.Legs,
			CreatedBy: actor.ID, CreatedByEmail: actor.Email,
		})
		if err != nil {
			return 0, nil, err
		}
		body, err := json.Marshal(j)
		return http.StatusCreated, body, err
	})
}

// Reverse posts a full compensating journal: every posting of the original with
// its direction flipped. A journal can be reversed once; reversals cannot be reversed.
func (s *LedgerService) Reverse(ctx context.Context, actor models.UserDTO, key, journalID string, req models.ReverseRequest) (models.IdempotentResult, error) {
	req.Description = strings.TrimSpace(req.Description)
	if utf8.RuneCountInString(req.Description) > 500 {
		return models.IdempotentResult{}, models.InvalidRequestError{Code: "invalid_reversal", Reason: "description must be at most 500 characters."}
	}
	hash, err := requestHash(http.MethodPost, "/api/v1/transfers/"+journalID+"/reverse", req)
	if err != nil {
		return models.IdempotentResult{}, err
	}
	return s.store.Idempotent(ctx, actor.ID, key, hash, func(ctx context.Context, w models.LedgerWriter) (int, []byte, error) {
		original, err := w.LockJournal(ctx, journalID)
		if err != nil {
			return 0, nil, err
		}
		if original.ReversalForID != nil {
			return 0, nil, ErrCannotReverseReversal
		}
		if original.ReversedByID != nil {
			return 0, nil, repositories.ErrAlreadyReversed
		}
		legs := make([]models.Leg, len(original.Postings))
		for i, p := range original.Postings {
			legs[i] = models.Leg{AccountID: p.AccountID, Direction: p.Direction.Opposite(), Amount: p.Amount}
		}
		description := req.Description
		if description == "" {
			description = truncate("Reversal of "+original.ReferenceID+": "+original.Description, 500)
		}
		j, err := w.PostJournal(ctx, models.NewJournal{
			ReferenceID: truncate("REV-"+original.ReferenceID, 128), Description: description, Currency: original.Currency,
			Legs: legs, ReversalForID: &original.ID, CreatedBy: actor.ID, CreatedByEmail: actor.Email,
		})
		if err != nil {
			return 0, nil, err
		}
		body, err := json.Marshal(j)
		return http.StatusCreated, body, err
	})
}

// requestHash fingerprints an idempotent request: method, route (with target IDs)
// and the canonical JSON of the validated body.
func requestHash(method, route string, body any) (string, error) {
	canonical, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(method + "\n" + route + "\n" + string(canonical)))
	return hex.EncodeToString(sum[:]), nil
}

func truncate(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}
