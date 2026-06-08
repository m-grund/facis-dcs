package db

import "errors"

var (
	// ErrPresentationNotFound is returned when no row exists for the presentation state.
	ErrPresentationNotFound = errors.New("presentation attempt not found")
	// ErrPresentationNotPending is returned when a state transition requires status pending.
	ErrPresentationNotPending = errors.New("presentation attempt is not pending")
)

// IsPresentationNotPending reports whether err is ErrPresentationNotPending.
func IsPresentationNotPending(err error) bool {
	return errors.Is(err, ErrPresentationNotPending)
}
