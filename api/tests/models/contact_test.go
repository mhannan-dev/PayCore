package models_test

import (
	"paycore/api/internal/models"
	"testing"
)

func TestUserAddressNormalize(t *testing.T) {
	blank, line2 := "  ", " Apt 4 "
	a := models.UserAddress{Label: " work ", Line1: " 1-2-3 Marunouchi ", Line2: &line2, City: " Tokyo ", PostalCode: &blank, CountryCode: "jp"}
	if err := a.Normalize(); err != nil {
		t.Fatal(err)
	}
	if a.Label != "WORK" || a.Line1 != "1-2-3 Marunouchi" || *a.Line2 != "Apt 4" || a.City != "Tokyo" || a.PostalCode != nil || a.CountryCode != "JP" {
		t.Fatalf("unexpected normalization: %+v", a)
	}
	if err := (&models.UserAddress{Line1: "x", City: "y", CountryCode: "JP"}).Normalize(); err != nil {
		t.Fatal("default label rejected:", err)
	}
	for _, bad := range []models.UserAddress{
		{Line1: "", City: "Tokyo", CountryCode: "JP"},
		{Line1: "x", City: " ", CountryCode: "JP"},
		{Line1: "x", City: "Tokyo", CountryCode: "JPN"},
		{Label: "VACATION", Line1: "x", City: "Tokyo", CountryCode: "JP"},
	} {
		if err := bad.Normalize(); err == nil {
			t.Fatalf("invalid address accepted: %+v", bad)
		}
	}
}

func TestUserContactNumberNormalize(t *testing.T) {
	c := models.UserContactNumber{PhoneNumber: " +81 (3) 1234-5678 "}
	if err := c.Normalize(); err != nil {
		t.Fatal(err)
	}
	if c.PhoneNumber != "+81312345678" || c.Label != "MOBILE" {
		t.Fatalf("unexpected normalization: %+v", c)
	}
	for _, bad := range []string{"", "0312345678", "+0123456789", "+1234", "+81312345678901234", "+81abc12345"} {
		if err := (&models.UserContactNumber{PhoneNumber: bad}).Normalize(); err == nil {
			t.Fatal("invalid phone accepted:", bad)
		}
	}
	if err := (&models.UserContactNumber{Label: "FAX", PhoneNumber: "+81312345678"}).Normalize(); err == nil {
		t.Fatal("unsupported label accepted")
	}
}
