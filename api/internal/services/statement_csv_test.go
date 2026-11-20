package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"paycore/api/internal/models"
)

const csvHeader = "booked_date,reference,currency,amount,direction,description\n"

func TestStatementAmounts(t *testing.T) {
	for _, tc := range []struct {
		s    string
		c    models.Currency
		want models.Amount
	}{
		{"12.50", models.CurrencyUSD, 1250}, {"00012.5", models.CurrencyUSD, 1250}, {"1", models.CurrencyJPY, 1},
		{"92233720368547758.07", models.CurrencyUSD, 9223372036854775807}, {"9223372036854775807", models.CurrencyJPY, 9223372036854775807},
		{"92233720368547758.08", models.CurrencyUSD, 0}, {"0.00", models.CurrencyUSD, 0}, {"-1", models.CurrencyUSD, 0},
		{"+1", models.CurrencyUSD, 0}, {"1e2", models.CurrencyUSD, 0}, {"1,000", models.CurrencyUSD, 0}, {"1.001", models.CurrencyUSD, 0},
		{"1.0", models.CurrencyJPY, 0}, {".50", models.CurrencyUSD, 0}, {"1.", models.CurrencyUSD, 0}, {"NaN", models.CurrencyUSD, 0},
	} {
		t.Run(string(tc.c)+tc.s, func(t *testing.T) {
			got, err := statementAmount(tc.s, tc.c)
			if got != tc.want || (err != nil) != (tc.want == 0) {
				t.Fatalf("got %v %v want %v", got, err, tc.want)
			}
		})
	}
}

func TestStatementCSVValidationAndDigest(t *testing.T) {
	ctx := context.Background()
	a, err := parseStatementCSV(ctx, []byte(csvHeader+"2026-11-20,ref-1,USD,12.50,CREDIT,receipt\n2026-11-19,ref-2,USD,2,DEBIT,fee\n"), models.CurrencyUSD)
	if err != nil || !a.CanImport || a.TotalCredits != "1250" || a.TotalDebits != "200" {
		t.Fatalf("preview %+v %v", a, err)
	}
	b, err := parseStatementCSV(ctx, []byte("\ufeffdescription,direction,amount,currency,reference,booked_date\r\nfee,debit,2.00,usd,ref-2,2026-11-19\r\nreceipt,credit,012.5,usd,ref-1,2026-11-20\r\n"), models.CurrencyUSD)
	if err != nil || b.ContentHash == nil || a.ContentHash == nil || *a.ContentHash != *b.ContentHash {
		t.Fatalf("normalized hash mismatch: %+v %v", b, err)
	}
	duplicate, err := parseStatementCSV(ctx, []byte(csvHeader+"2026-11-20,ref,USD,1,CREDIT,\n2026-11-20,ref,USD,1.00,CREDIT,\n"), models.CurrencyUSD)
	if err != nil || duplicate.InvalidRows != 1 || duplicate.CanImport || duplicate.ContentHash != nil {
		t.Fatalf("duplicate accepted %+v %v", duplicate, err)
	}
	for _, record := range []string{
		"2026-02-30,ref,USD,1,CREDIT,", "2026-11-20,,USD,1,CREDIT,", "2026-11-20,ref,EUR,1,CREDIT,",
		"2026-11-20,ref,USD,0,CREDIT,", "2026-11-20,ref,USD,1,OTHER,", "1899-12-31,ref,USD,1,CREDIT,",
		"2026-11-20," + strings.Repeat("x", 129) + ",USD,1,CREDIT,", "2026-11-20,ref,USD,1,CREDIT," + strings.Repeat("x", 501),
	} {
		p, err := parseStatementCSV(ctx, []byte(csvHeader+record+"\n"), models.CurrencyUSD)
		if err != nil || p.InvalidRows != 1 || p.Rows[0].Entry != nil {
			t.Fatalf("invalid row accepted: %.100s %+v %v", record, p, err)
		}
	}
	quoted, err := parseStatementCSV(ctx, []byte(csvHeader+"2026-11-20,ref,USD,1,CREDIT,\"quoted, value\nsecond line\"\n"), models.CurrencyUSD)
	if err != nil || !quoted.CanImport {
		t.Fatalf("quoted CSV failed %v", err)
	}
}

func TestStatementCSVBoundsAndMalformedInput(t *testing.T) {
	for _, data := range []string{"", csvHeader, "date,reference,currency,amount,direction,description\n", "booked_date,reference,currency,amount,direction,direction\n", csvHeader + "a,b\n", csvHeader + "\"unterminated", csvHeader + string([]byte{0xff}), strings.Repeat("a", models.StatementMaxBytes+1)} {
		if _, err := parseStatementCSV(context.Background(), []byte(data), models.CurrencyUSD); !errors.Is(err, models.ErrStatementFormat) {
			t.Fatalf("want format error for %.70q got %v", data, err)
		}
	}
	var b strings.Builder
	b.WriteString(csvHeader)
	for i := 0; i < models.StatementMaxRows+1; i++ {
		fmt.Fprintf(&b, "2026-11-20,ref-%d,USD,1,CREDIT,\n", i)
	}
	if _, err := parseStatementCSV(context.Background(), []byte(b.String()), models.CurrencyUSD); !errors.Is(err, models.ErrStatementFormat) {
		t.Fatalf("row limit: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := parseStatementCSV(ctx, []byte(csvHeader+"2026-11-20,ref,USD,1,CREDIT,\n"), models.CurrencyUSD); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestStatementTotalsCanExceedInt64(t *testing.T) {
	p, err := parseStatementCSV(context.Background(), []byte(csvHeader+"2026-11-20,A,JPY,9223372036854775807,CREDIT,\n2026-11-20,B,JPY,9223372036854775807,CREDIT,\n"), models.CurrencyJPY)
	if err != nil || p.TotalCredits != "18446744073709551614" || !p.CanImport {
		t.Fatalf("totals overflowed %+v %v", p, err)
	}
}
