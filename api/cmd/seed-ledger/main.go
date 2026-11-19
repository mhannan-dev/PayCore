// Command seed-ledger creates a small development chart of accounts and posts the
// sample journals the web console used to show as fixtures, through the real
// ledger service (locking, overdraft checks, idempotency).
//
//	cd api; go run ./cmd/seed-ledger
//
// Re-running is safe: existing account numbers are reused and every journal uses
// a fixed Idempotency-Key, so it is replayed instead of posted twice. Journals are
// append-only; to remove them, rebuild the development database.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"paycore/api/internal/config"
	"paycore/api/internal/database"
	"paycore/api/internal/model"
	"paycore/api/internal/repository"
	"paycore/api/internal/service"
	"paycore/api/migrations"
)

// seedActor scopes the seed's idempotency keys; it is not a real user.
var seedActor = model.UserDTO{ID: "00000000-0000-4000-8000-000000000001", Email: "seed-ledger@paycore.local"}

type accountSeed struct {
	number, name string
	typ          model.AccountType
}

var chart = []accountSeed{
	{"1001", "Operating bank", model.AccountAsset},
	{"2001", "Akira · Wallet", model.AccountLiability},
	{"2002", "Hana · Wallet", model.AccountLiability},
	{"2003", "Payout clearing", model.AccountLiability},
	{"4001", "Platform fee revenue", model.AccountRevenue},
}

type journalSeed struct {
	ref, description, debit, credit string
	amount                          model.Amount
}

var journals = []journalSeed{
	{"JRN-0001", "Akira wallet funding", "1001", "2001", 500000},
	{"JRN-0002", "Hana wallet funding", "1001", "2002", 300000},
	{"JRN-0003", "Akira → Hana wallet transfer", "2001", "2002", 50000},
	{"JRN-0004", "Wallet transfer platform fee", "2002", "4001", 500},
	{"JRN-0005", "Reserve funds for payout", "2002", "2003", 25000},
	{"JRN-0006", "Sandbox payout captured", "2003", "1001", 25000},
}

func main() {
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal("Database connection failed.")
	}
	defer pool.Close()
	if err := migrations.Apply(ctx, pool); err != nil {
		log.Fatalf("Database migration failed: %v", err)
	}
	repo := repository.NewLedgerRepository(pool)
	ledger := service.NewLedgerService(repo)

	ids := map[string]string{}
	for _, a := range chart {
		account, err := ledger.CreateAccount(ctx, model.UserDTO{}, model.CreateAccountRequest{
			AccountNumber: a.number, Name: a.name, Type: a.typ, Currency: model.CurrencyJPY,
		})
		status := "created"
		if errors.Is(err, repository.ErrAccountNumberTaken) {
			account, err = findAccount(ctx, ledger, a.number)
			status = "exists"
		}
		if err != nil {
			log.Fatalf("account %s: %v", a.number, err)
		}
		ids[a.number] = account.ID
		fmt.Printf("account %-5s %-22s %-9s %s\n", a.number, a.name, a.typ, status)
	}

	for _, j := range journals {
		result, err := ledger.Transfer(ctx, seedActor, "seed-ledger-"+j.ref, model.TransferRequest{
			ReferenceID: j.ref, Description: j.description, Currency: model.CurrencyJPY,
			Legs: []model.Leg{
				{AccountID: ids[j.debit], Direction: model.Debit, Amount: j.amount},
				{AccountID: ids[j.credit], Direction: model.Credit, Amount: j.amount},
			},
		})
		if err != nil {
			log.Fatalf("journal %s: %v", j.ref, err)
		}
		status := "posted"
		if result.Replayed {
			status = "already posted"
		}
		fmt.Printf("journal %s %-30s ¥%-8s %s\n", j.ref, j.description, j.amount, status)
	}

	report, err := ledger.ZeroSum(ctx, model.CurrencyJPY)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("zero-sum JPY: debits %s, credits %s, balanced=%t\n", report.TotalDebits, report.TotalCredits, report.Balanced)
}

func findAccount(ctx context.Context, ledger *service.LedgerService, number string) (model.Account, error) {
	accounts, _, err := ledger.Accounts(ctx, model.AccountQuery{Search: number, Limit: 50})
	if err != nil {
		return model.Account{}, err
	}
	for _, a := range accounts {
		if a.AccountNumber == number {
			return a, nil
		}
	}
	return model.Account{}, repository.ErrAccountNotFound
}
