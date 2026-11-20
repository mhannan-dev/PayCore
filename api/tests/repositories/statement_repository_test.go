package repositories_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"paycore/api/internal/models"
	"paycore/api/internal/repositories"
)

func TestStatementImportAtomicityOwnershipAndConcurrentDeduplication(t *testing.T) {
	f := newLedgerFixture(t)
	ctx := context.Background()
	users := repositories.NewUserRepository(f.pool)
	owner, err := users.Create(ctx, "Importer", nil, "importer@example.test", "fixture hash", []string{models.RoleOperator})
	if err != nil {
		t.Fatal(err)
	}
	other, err := users.Create(ctx, "Other", nil, "other@example.test", "fixture hash", []string{models.RoleOperator})
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Repeat("a", 64)
	key := strings.Repeat("b", 64)
	var attachment string
	if err = f.pool.QueryRow(ctx, `INSERT INTO attachments(owner_user_id,file_name,content_type,size_bytes,sha256,storage_kind,storage_key) VALUES($1,'statement.csv','text/csv',100,$2,'LOCAL',$3) RETURNING id`, owner.ID, source, key+".csv").Scan(&attachment); err != nil {
		t.Fatal(err)
	}
	hash := strings.Repeat("c", 64)
	p := models.StatementPreview{AttachmentID: attachment, AccountID: f.usdCash.ID, FileName: "statement.csv", Currency: models.CurrencyUSD, ContentHash: &hash, TotalDebits: "1250", TotalCredits: "0", Rows: []models.StatementRow{{RowNumber: 2, Entry: &models.StatementEntry{BookedDate: "2026-11-20", Reference: "ref", Currency: models.CurrencyUSD, Amount: 1250, Direction: models.Debit, Description: "bank fee"}}}}
	r := repositories.NewStatementRepository(f.pool)
	if _, err = r.Save(ctx, other.ID, p, source); !errors.Is(err, models.ErrStatementChanged) {
		t.Fatalf("foreign source accepted %v", err)
	}
	// A rejected audit insert must roll back the import and all rows.
	if _, err = f.pool.Exec(ctx, `ALTER TABLE audit_events ADD CONSTRAINT test_no_statement CHECK(action<>'statement_imported') NOT VALID`); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Save(ctx, owner.ID, p, source); err == nil {
		t.Fatal("audit failure did not abort import")
	}
	var count int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM statement_imports`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial import persisted %d %v", count, err)
	}
	if _, err = f.pool.Exec(ctx, `ALTER TABLE audit_events DROP CONSTRAINT test_no_statement`); err != nil {
		t.Fatal(err)
	}
	// Two writers racing from the same preview yield exactly one committed import.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	ids := make(chan string, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := r.Save(ctx, owner.ID, p, source)
			results <- err
			if err == nil {
				ids <- v.ID
			}
		}()
	}
	wg.Wait()
	close(results)
	close(ids)
	success, duplicate := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, models.ErrStatementDuplicate) {
			duplicate++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || duplicate != 1 {
		t.Fatalf("success %d duplicates %d", success, duplicate)
	}
	id := <-ids
	detail, err := r.Detail(ctx, owner.ID, id, 50, 0)
	if err != nil || len(detail.Rows) != 1 || detail.Rows[0].Amount != 1250 {
		t.Fatalf("detail %+v %v", detail, err)
	}
	if _, err = r.Detail(ctx, other.ID, id, 50, 0); !errors.Is(err, models.ErrStatementNotFound) {
		t.Fatalf("foreign detail visible %v", err)
	}
	list, err := r.List(ctx, other.ID, 20, 0)
	if err != nil || len(list) != 0 {
		t.Fatalf("foreign history visible %v %v", list, err)
	}
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE action='statement_imported'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("audit count %d %v", count, err)
	}
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM journal_entries`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("import posted journal entries %d %v", count, err)
	}
	// A later source deletion doesn't remove already imported financial rows.
	if _, err = f.pool.Exec(ctx, `DELETE FROM attachments WHERE id=$1`, attachment); err != nil {
		t.Fatal(err)
	}
	detail, err = r.Detail(ctx, owner.ID, id, 50, 0)
	if err != nil || detail.Import.AttachmentID != nil || len(detail.Rows) != 1 {
		t.Fatalf("retention %+v %v", detail, err)
	}
	hash = strings.Repeat("d", 64)
	if _, err = r.Save(ctx, owner.ID, p, source); !errors.Is(err, models.ErrStatementChanged) {
		t.Fatalf("deleted source accepted %v", err)
	}
}
