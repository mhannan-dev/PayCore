package services

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"paycore/api/internal/models"
)

var statementHeaders = [...]string{"booked_date", "reference", "currency", "amount", "direction", "description"}

// parseStatementCSV bounds both work and output. Money is parsed using decimal
// strings, and the digest is independent of file name, row order and formatting.
func parseStatementCSV(ctx context.Context, data []byte, currency models.Currency) (models.StatementPreview, error) {
	p := models.StatementPreview{Currency: currency, Rows: []models.StatementRow{}, TotalDebits: "0", TotalCredits: "0"}
	if len(data) > models.StatementMaxBytes || !utf8.Valid(data) {
		return p, models.ErrStatementFormat
	}
	r := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(data), "\ufeff")))
	header, err := r.Read()
	if err != nil || len(header) != len(statementHeaders) {
		return p, models.ErrStatementFormat
	}
	indexes := make(map[string]int, 6)
	for i, h := range header {
		h = strings.ToLower(strings.TrimSpace(h))
		if _, ok := indexes[h]; ok {
			return p, models.ErrStatementFormat
		}
		indexes[h] = i
	}
	for _, h := range statementHeaders {
		if _, ok := indexes[h]; !ok {
			return p, models.ErrStatementFormat
		}
	}
	debit, credit := new(big.Int), new(big.Int)
	hashes := []string{}
	seen := map[string]bool{}
	for {
		if err := ctx.Err(); err != nil {
			return p, err
		}
		record, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return p, models.ErrStatementFormat
		}
		if len(p.Rows) >= models.StatementMaxRows {
			return p, models.ErrStatementFormat
		}
		line, _ := r.FieldPos(0)
		v := make([]string, 6)
		for i, h := range statementHeaders {
			v[i] = strings.TrimSpace(record[indexes[h]])
		}
		row := models.StatementRow{RowNumber: line, Values: make([]string, 6), Errors: []string{}}
		for i, s := range v {
			rr := []rune(s)
			if len(rr) > 512 {
				row.Values[i] = string(rr[:512]) + "…"
			} else {
				row.Values[i] = s
			}
		}
		e := models.StatementEntry{BookedDate: v[0], Reference: v[1], Currency: models.Currency(strings.ToUpper(v[2])), Direction: models.Direction(strings.ToUpper(v[4])), Description: v[5]}
		d, err := time.Parse("2006-01-02", v[0])
		if err != nil || d.Year() < 1900 || d.Format("2006-01-02") != v[0] {
			row.Errors = append(row.Errors, "booked_date: use YYYY-MM-DD (1900 or later)")
		}
		if !statementText(v[1], 128, false) || v[1] == "" {
			row.Errors = append(row.Errors, "reference: required, at most 128 characters, no control characters")
		}
		if !statementText(v[5], 500, true) {
			row.Errors = append(row.Errors, "description: at most 500 characters, no unsupported control characters")
		}
		if !e.Currency.Valid() || e.Currency != currency {
			row.Errors = append(row.Errors, "currency: must match the selected account")
		}
		e.Amount, err = statementAmount(v[3], currency)
		if err != nil {
			row.Errors = append(row.Errors, "amount: positive decimal within int64 minor units; no signs, separators or excess decimal places")
		}
		if !e.Direction.Valid() {
			row.Errors = append(row.Errors, "direction: use DEBIT or CREDIT")
		}
		if len(row.Errors) == 0 {
			encoded, marshalErr := json.Marshal(e)
			if marshalErr != nil {
				return models.StatementPreview{}, fmt.Errorf("fingerprint statement row: %w", marshalErr)
			}
			sum := sha256.Sum256(encoded)
			hash := hex.EncodeToString(sum[:])
			if seen[hash] {
				row.Errors = append(row.Errors, "duplicate row within this file")
			} else {
				seen[hash] = true
				hashes = append(hashes, hash)
				row.Entry = &e
				total := debit
				if e.Direction == models.Credit {
					total = credit
				}
				total.Add(total, big.NewInt(int64(e.Amount)))
			}
		}
		if len(row.Errors) > 0 {
			p.InvalidRows++
		}
		p.Rows = append(p.Rows, row)
	}
	if len(p.Rows) == 0 {
		return p, models.ErrStatementFormat
	}
	p.TotalDebits = debit.String()
	p.TotalCredits = credit.String()
	if p.InvalidRows == 0 {
		sort.Strings(hashes)
		sum := sha256.Sum256([]byte(strings.Join(hashes, "\n")))
		hash := hex.EncodeToString(sum[:])
		p.ContentHash = &hash
		p.CanImport = true
	}
	return p, nil
}

func statementText(s string, max int, multiline bool) bool {
	if utf8.RuneCountInString(s) > max {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) && !(multiline && (r == '\n' || r == '\r' || r == '\t')) {
			return false
		}
	}
	return true
}

func statementAmount(s string, c models.Currency) (models.Amount, error) {
	if !c.Valid() || len(s) > 30 {
		return 0, models.ErrStatementInvalid
	}
	parts := strings.Split(s, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, models.ErrStatementInvalid
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
		if fraction == "" || c.Scale() == 0 {
			return 0, models.ErrStatementInvalid
		}
	}
	if len(fraction) > c.Scale() {
		return 0, models.ErrStatementInvalid
	}
	for _, r := range parts[0] + fraction {
		if r < '0' || r > '9' {
			return 0, models.ErrStatementInvalid
		}
	}
	minor := strings.TrimLeft(parts[0]+fraction+strings.Repeat("0", c.Scale()-len(fraction)), "0")
	if _, err := strconv.ParseInt(minor, 10, 64); err != nil {
		return 0, models.ErrStatementInvalid
	}
	return models.ParseAmount(minor)
}
