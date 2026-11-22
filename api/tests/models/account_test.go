package models_test

import (
	"testing"

	"paycore/api/internal/models"
)

func TestCreateAccountSystemRole(t *testing.T) {
	role := func(s string) *models.AccountSystemRole { r := models.AccountSystemRole(s); return &r }
	cases := []struct {
		name  string
		typ   models.AccountType
		role  *models.AccountSystemRole
		ok    bool
		wants *models.AccountSystemRole // Expected after Normalize; nil means an ordinary account.
	}{
		{"absent", models.AccountLiability, nil, true, nil},
		{"empty means none", models.AccountLiability, role("  "), true, nil},
		{"clearing on liability", models.AccountLiability, role(" payout_clearing "), true, role("PAYOUT_CLEARING")},
		{"bank on asset", models.AccountAsset, role("OPERATING_BANK"), true, role("OPERATING_BANK")},
		{"clearing on asset", models.AccountAsset, role("PAYOUT_CLEARING"), false, nil},
		{"bank on liability", models.AccountLiability, role("OPERATING_BANK"), false, nil},
		{"unknown role", models.AccountAsset, role("TREASURY"), false, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := models.CreateAccountRequest{AccountNumber: "2003", Name: "Account", Type: tc.typ, Currency: models.CurrencyJPY, SystemRole: tc.role}
			err := req.Normalize()
			if (err == nil) != tc.ok {
				t.Fatalf("Normalize error = %v, want ok=%v", err, tc.ok)
			}
			if !tc.ok {
				return
			}
			if (req.SystemRole == nil) != (tc.wants == nil) || (req.SystemRole != nil && *req.SystemRole != *tc.wants) {
				t.Fatalf("SystemRole = %v, want %v", req.SystemRole, tc.wants)
			}
		})
	}
}
