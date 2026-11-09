// Package errs marks errors whose message is meant for the person using
// Hayat: "this slug is taken", not "connection refused". APIs pass these
// messages through and replace everything else with a generic one.
package errs

import (
	"errors"
	"fmt"
)

type Error struct{ msg string }

func (e *Error) Error() string { return e.msg }

// New makes a user-facing error. Declare sentinels with it; errors.Is matches
// them by identity.
func New(msg string) *Error { return &Error{msg: msg} }

func Newf(format string, args ...any) *Error { return &Error{msg: fmt.Sprintf(format, args...)} }

// Is reports whether err (or anything it wraps) is user-facing.
func Is(err error) bool {
	var e *Error
	return errors.As(err, &e)
}
