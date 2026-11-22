package models_test

import (
	"paycore/api/internal/models"
	"testing"
)

func TestNormalizeRoles(t *testing.T) {
	roles, err := models.NormalizeRoles([]string{" admin ", "accountant", "ADMIN"})
	if err != nil || len(roles) != 2 || roles[0] != "ADMIN" || roles[1] != "ACCOUNTANT" {
		t.Fatal("normalization/deduplication failed")
	}
	for _, invalid := range [][]string{nil, {}, {""}, {"UNKNOWN"}, {"ADMIN", ""}, {"OPERATOR"}, {"LEAD_OPERATOR"}, {"STAFF_ENGINEER"}} {
		if _, err := models.NormalizeRoles(invalid); err == nil {
			t.Fatal("invalid roles accepted")
		}
	}
	u := models.User{Roles: roles}
	if !u.HasRole("ADMIN") || !u.HasRole("ACCOUNTANT") || u.HasRole("OPERATOR") || u.HasRole("LEAD_OPERATOR") || u.HasRole("STAFF_ENGINEER") {
		t.Fatal("role membership failed")
	}
	dto := u.Public()
	dto.Roles[0] = "OPERATOR"
	if !u.HasRole("ADMIN") {
		t.Fatal("DTO role mutation changed User")
	}
}
