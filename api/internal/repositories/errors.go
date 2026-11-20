package repositories

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"paycore/api/internal/models"
)

// Not-found errors name what was missing, so the HTTP layer can answer
// "user_not_found" or "attachment_not_found". They wrap pgx.ErrNoRows, so
// errors.Is(err, pgx.ErrNoRows) still holds for existing callers.
var (
	ErrUserNotFound       = fmt.Errorf("user not found: %w", pgx.ErrNoRows)
	ErrAttachmentNotFound = fmt.Errorf("%w: %w", models.ErrAttachmentNotFound, pgx.ErrNoRows)
)

// notFound replaces a bare pgx.ErrNoRows with sentinel; other errors pass through.
func notFound(err, sentinel error) error {
	if errors.Is(err, pgx.ErrNoRows) && !errors.Is(err, sentinel) {
		return sentinel
	}
	return err
}
