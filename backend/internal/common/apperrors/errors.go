package apperrors

import "errors"

var (
	ErrForbidden = errors.New("forbidden")
	ErrNotFound  = errors.New("not_found")
	ErrConflict  = errors.New("conflict")
	ErrInvalid   = errors.New("invalid")
)
