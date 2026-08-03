package billing

import "errors"

var (
	ErrNotFound = errors.New("billing: not found")
	ErrConflict = errors.New("billing: conflict")
)
