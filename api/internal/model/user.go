package model

import "time"

// User is a stored login identity, distinct from a financial ledger account.
type User struct {
	ID                string    `json:"id"`
	FirstName         string    `json:"first_name"`
	LastName          *string   `json:"last_name"`
	Photo             *string   `json:"photo"`
	PhotoAttachmentID *string   `json:"photo_attachment_id"`
	Email             string    `json:"email"`
	PasswordHash      string    `json:"-"`
	Roles             []string  `json:"roles"`
	Permissions       []string  `json:"permissions"`
	IsActive          bool      `json:"is_active"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (u User) Public() UserDTO {
	return UserDTO{ID: u.ID, FirstName: u.FirstName, LastName: u.LastName, Photo: u.Photo, PhotoAttachmentID: u.PhotoAttachmentID, Email: u.Email, Roles: append([]string{}, u.Roles...), Permissions: append([]string{}, u.Permissions...)}
}
