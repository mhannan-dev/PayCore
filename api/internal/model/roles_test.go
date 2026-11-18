package model

import "testing"

func TestNormalizeRoles(t *testing.T) {
	roles, err := NormalizeRoles([]string{" admin ", "operator", "ADMIN"})
	if err != nil || len(roles) != 2 || roles[0] != "ADMIN" || roles[1] != "OPERATOR" {
		t.Fatal("normalization/deduplication failed")
	}
	for _, invalid := range [][]string{nil, {}, {""}, {"UNKNOWN"}, {"ADMIN", ""}} {
		if _, err := NormalizeRoles(invalid); err == nil {
			t.Fatal("invalid roles accepted")
		}
	}
	u := User{Roles: roles}
	if !u.HasRole("ADMIN") || !u.HasRole("OPERATOR") || u.HasRole("STAFF_ENGINEER") {
		t.Fatal("role membership failed")
	}
	dto := u.Public()
	dto.Roles[0] = "STAFF_ENGINEER"
	if !u.HasRole("ADMIN") {
		t.Fatal("DTO role mutation changed User")
	}
}
