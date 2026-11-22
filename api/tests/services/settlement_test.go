package services_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"paycore/api/internal/models"
	"paycore/api/internal/repositories"
	"paycore/api/internal/services"
)

const sourceID = "22222222-2222-4222-8222-222222222222"
const clearingID = "33333333-3333-4333-8333-333333333333"
const bankID = "44444444-4444-4444-8444-444444444444"

type payoutRecord struct {
	hash   string
	result models.IdempotentResult
}
type fakeSettlementStore struct {
	mu       sync.Mutex
	items    map[string]models.Settlement
	records  map[string]payoutRecord
	accounts []models.Account
	journals []models.NewJournal
	writeErr error
	postErr  error
}

func newFakeSettlementStore() *fakeSettlementStore {
	return &fakeSettlementStore{items: map[string]models.Settlement{}, records: map[string]payoutRecord{}, accounts: []models.Account{
		{ID: sourceID, AccountNumber: "2002", Name: "Customer wallet", Type: models.AccountLiability, Currency: models.CurrencyJPY, IsActive: true},
		{ID: clearingID, AccountNumber: "2003", Name: "Payout clearing", Type: models.AccountLiability, Currency: models.CurrencyJPY, IsActive: true},
		{ID: bankID, AccountNumber: "1001", Name: "Operating bank", Type: models.AccountAsset, Currency: models.CurrencyJPY, IsActive: true},
	}}
}
func (s *fakeSettlementStore) GetByID(_ context.Context, id string) (models.Settlement, error) {
	return s.items[id], nil
}
func (s *fakeSettlementStore) List(_ context.Context, q models.SettlementQuery) ([]models.Settlement, int, error) {
	items := []models.Settlement{}
	for _, item := range s.items {
		if q.Currency == "" || q.Currency == item.Currency {
			items = append(items, item)
		}
	}
	return items, len(items), nil
}
func (s *fakeSettlementStore) Execute(ctx context.Context, actor, key, hash string, op func(context.Context, models.SettlementWriter) (int, []byte, error)) (models.IdempotentResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if record, ok := s.records[actor+key]; ok {
		if record.hash != hash {
			return models.IdempotentResult{}, repositories.ErrIdempotencyMismatch
		}
		result := record.result
		result.Replayed = true
		return result, nil
	}
	items := make(map[string]models.Settlement)
	for id, item := range s.items {
		items[id] = item
	}
	writer := &fakeSettlementWriter{store: s, items: items}
	status, body, err := op(ctx, writer)
	if err != nil {
		return models.IdempotentResult{}, err
	}
	s.items = items
	s.journals = append(s.journals, writer.journals...)
	result := models.IdempotentResult{Status: status, Body: body}
	s.records[actor+key] = payoutRecord{hash: hash, result: result}
	return result, nil
}

type fakeSettlementWriter struct {
	store    *fakeSettlementStore
	items    map[string]models.Settlement
	journals []models.NewJournal
}

func (w *fakeSettlementWriter) Accounts(_ context.Context, currency models.Currency) ([]models.Account, error) {
	items := []models.Account{}
	for _, account := range w.store.accounts {
		if account.Currency == currency {
			items = append(items, account)
		}
	}
	return items, nil
}
func (w *fakeSettlementWriter) GetSettlement(_ context.Context, id string) (models.Settlement, error) {
	return w.items[id], nil
}
func (w *fakeSettlementWriter) CreateSettlement(_ context.Context, payout models.Settlement) error {
	if w.store.writeErr != nil {
		return w.store.writeErr
	}
	w.items[payout.ID] = payout
	return nil
}
func (w *fakeSettlementWriter) UpdateSettlement(ctx context.Context, payout models.Settlement) error {
	return w.CreateSettlement(ctx, payout)
}
func (w *fakeSettlementWriter) PostJournal(_ context.Context, journal models.NewJournal) (models.Journal, error) {
	if w.store.postErr != nil {
		return models.Journal{}, w.store.postErr
	}
	w.journals = append(w.journals, journal)
	return models.Journal{ID: fmt.Sprintf("55555555-5555-4555-8555-%012d", len(w.store.journals)+len(w.journals))}, nil
}
func payoutRequest() models.CreatePayoutRequest {
	return models.CreatePayoutRequest{SourceAccountID: sourceID, Amount: "25000", Currency: models.CurrencyJPY, BeneficiaryName: "Hana", BeneficiaryBank: "Mizuho", BeneficiaryAccount: "00123456"}
}
func payoutResult(t *testing.T, result models.IdempotentResult, err error) models.Settlement {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	var payout models.Settlement
	if err := json.Unmarshal(result.Body, &payout); err != nil {
		t.Fatal(err)
	}
	return payout
}
func reservePayout(t *testing.T, svc *services.SettlementService) models.Settlement {
	t.Helper()
	result, err := svc.Reserve(context.Background(), models.UserDTO{ID: "actor"}, "reserve-key", payoutRequest())
	return payoutResult(t, result, err)
}
func TestSettlementLifecycle(t *testing.T) {
	store := newFakeSettlementStore()
	svc := services.NewSettlementService(store)
	actor := models.UserDTO{ID: "actor"}
	ctx := context.Background()
	payout := reservePayout(t, svc)
	if !models.ValidUUID(payout.ID) || payout.Status != models.SettlementReserved || len(store.journals) != 1 {
		t.Fatalf("reserve: %+v", payout)
	}
	replay, err := svc.Reserve(ctx, actor, "reserve-key", payoutRequest())
	if err != nil || !replay.Replayed || len(store.journals) != 1 {
		t.Fatalf("replay: %+v %v", replay, err)
	}
	changed := payoutRequest()
	changed.Amount = "25001"
	if _, err := svc.Reserve(ctx, actor, "reserve-key", changed); !errors.Is(err, repositories.ErrIdempotencyMismatch) {
		t.Fatalf("changed intent: %v", err)
	}
	result, err := svc.Capture(ctx, actor, "capture-key", payout.ID)
	captured := payoutResult(t, result, err)
	if captured.Status != models.SettlementCaptured || captured.DestinationAccountID == nil || *captured.DestinationAccountID != bankID || captured.CaptureJournalID == nil || len(store.journals) != 2 {
		t.Fatalf("capture: %+v", captured)
	}
	replay, err = svc.Capture(ctx, actor, "capture-key", payout.ID)
	if err != nil || !replay.Replayed || len(store.journals) != 2 {
		t.Fatalf("capture replay: %v", err)
	}
	if _, err := svc.Release(ctx, actor, "release-key", payout.ID); err == nil {
		t.Fatal("captured payout was released")
	}
}
func TestSettlementRelease(t *testing.T) {
	store := newFakeSettlementStore()
	svc := services.NewSettlementService(store)
	payout := reservePayout(t, svc)
	result, err := svc.Release(context.Background(), models.UserDTO{ID: "actor"}, "release-key", payout.ID)
	released := payoutResult(t, result, err)
	if released.Status != models.SettlementReleased || released.ReleaseJournalID == nil || released.CaptureJournalID != nil || len(store.journals) != 2 {
		t.Fatalf("release: %+v", released)
	}
	if store.journals[1].Legs[1].AccountID != sourceID {
		t.Fatal("release did not refund the source wallet")
	}
}
func TestSettlementFailureRollsBack(t *testing.T) {
	store := newFakeSettlementStore()
	svc := services.NewSettlementService(store)
	store.writeErr = errors.New("database write failed")
	if _, err := svc.Reserve(context.Background(), models.UserDTO{ID: "actor"}, "reserve-key", payoutRequest()); err == nil || len(store.journals) != 0 || len(store.items) != 0 {
		t.Fatal("failed reservation committed money")
	}
	store.writeErr = nil
	payout := reservePayout(t, svc)
	store.writeErr = errors.New("status update failed")
	if _, err := svc.Capture(context.Background(), models.UserDTO{ID: "actor"}, "capture-key", payout.ID); err == nil || len(store.journals) != 1 || store.items[payout.ID].Status != models.SettlementReserved {
		t.Fatal("failed capture committed money or state")
	}
}
func TestSettlementCompetingActions(t *testing.T) {
	store := newFakeSettlementStore()
	svc := services.NewSettlementService(store)
	payout := reservePayout(t, svc)
	results := make(chan error, 2)
	go func() {
		_, err := svc.Capture(context.Background(), models.UserDTO{ID: "actor"}, "capture-key", payout.ID)
		results <- err
	}()
	go func() {
		_, err := svc.Release(context.Background(), models.UserDTO{ID: "actor"}, "release-key", payout.ID)
		results <- err
	}()
	successes := 0
	for range 2 {
		if <-results == nil {
			successes++
		}
	}
	if successes != 1 || len(store.journals) != 2 {
		t.Fatalf("competing transitions: %d successes, %d journals", successes, len(store.journals))
	}
}
func TestSettlementRejectsInvalidSources(t *testing.T) {
	for _, id := range []string{clearingID, bankID, "66666666-6666-4666-8666-666666666666"} {
		store := newFakeSettlementStore()
		svc := services.NewSettlementService(store)
		req := payoutRequest()
		req.SourceAccountID = id
		if _, err := svc.Reserve(context.Background(), models.UserDTO{ID: "actor"}, "reserve-key", req); err == nil || len(store.journals) != 0 {
			t.Fatalf("invalid source %s accepted", id)
		}
	}
	store := newFakeSettlementStore()
	store.accounts[0].IsActive = false
	if _, err := services.NewSettlementService(store).Reserve(context.Background(), models.UserDTO{ID: "actor"}, "reserve-key", payoutRequest()); err == nil {
		t.Fatal("inactive wallet accepted")
	}
}
