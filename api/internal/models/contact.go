package models

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// UserAddress is one of a user's postal addresses (table user_addresses).
type UserAddress struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	Label       string    `json:"label"`
	Line1       string    `json:"line1"`
	Line2       *string   `json:"line2"`
	City        string    `json:"city"`
	StateRegion *string   `json:"state_region"`
	PostalCode  *string   `json:"postal_code"`
	CountryCode string    `json:"country_code"`
	IsPrimary   bool      `json:"is_primary"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// UserContactNumber is one of a user's phone numbers (table user_contact_numbers).
type UserContactNumber struct {
	ID          string     `json:"id"`
	UserID      string     `json:"user_id"`
	Label       string     `json:"label"`
	PhoneNumber string     `json:"phone_number"`
	IsPrimary   bool       `json:"is_primary"`
	VerifiedAt  *time.Time `json:"verified_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

var (
	countryCodePattern = regexp.MustCompile(`^[A-Z]{2}$`)
	e164Pattern        = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)
	phoneSeparators    = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "", ".", "")
)

func trimmedOptional(value *string, max int) (*string, error) {
	if value == nil {
		return nil, nil
	}
	v := strings.TrimSpace(*value)
	if v == "" {
		return nil, nil
	}
	if len([]rune(v)) > max {
		return nil, errors.New("field is too long")
	}
	return &v, nil
}

func requiredText(value string, max int, field string) (string, error) {
	v := strings.TrimSpace(value)
	if v == "" || len([]rune(v)) > max {
		return "", errors.New(field + " is required and must be at most " + strconv.Itoa(max) + " characters")
	}
	return v, nil
}

// Normalize trims and uppercases fields and checks the same rules the
// database enforces, so callers get a clear error before hitting a constraint.
func (a *UserAddress) Normalize() error {
	a.Label = strings.ToUpper(strings.TrimSpace(a.Label))
	if a.Label == "" {
		a.Label = "HOME"
	}
	switch a.Label {
	case "HOME", "WORK", "BILLING", "MAILING", "OTHER":
	default:
		return errors.New("unsupported address label")
	}
	var err error
	if a.Line1, err = requiredText(a.Line1, 200, "line1"); err != nil {
		return err
	}
	if a.City, err = requiredText(a.City, 100, "city"); err != nil {
		return err
	}
	if a.Line2, err = trimmedOptional(a.Line2, 200); err != nil {
		return err
	}
	if a.StateRegion, err = trimmedOptional(a.StateRegion, 100); err != nil {
		return err
	}
	if a.PostalCode, err = trimmedOptional(a.PostalCode, 20); err != nil {
		return err
	}
	a.CountryCode = strings.ToUpper(strings.TrimSpace(a.CountryCode))
	if !countryCodePattern.MatchString(a.CountryCode) {
		return errors.New("country_code must be an ISO 3166-1 alpha-2 code")
	}
	return nil
}

// Normalize strips common separators and requires E.164 format (e.g. +81312345678).
func (c *UserContactNumber) Normalize() error {
	c.Label = strings.ToUpper(strings.TrimSpace(c.Label))
	if c.Label == "" {
		c.Label = "MOBILE"
	}
	switch c.Label {
	case "MOBILE", "HOME", "WORK", "OTHER":
	default:
		return errors.New("unsupported contact number label")
	}
	c.PhoneNumber = phoneSeparators.Replace(strings.TrimSpace(c.PhoneNumber))
	if !e164Pattern.MatchString(c.PhoneNumber) {
		return errors.New("phone_number must be in E.164 format, e.g. +81312345678")
	}
	return nil
}
