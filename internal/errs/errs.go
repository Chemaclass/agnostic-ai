// Package errs defines stable, user-facing error codes and a small
// constructor for tagging errors with them. Codes follow the
// `AAI-NNN` shape and are stable across releases so users can grep,
// link to docs, and run `agnostic-ai explain <code>` for guidance.
//
// The constructor produces errors whose Error() begins with `[AAI-NNN] `
// and whose underlying chain (when any %w is supplied) is preserved so
// errors.Is / errors.As keep working unchanged for callers that already
// match against sentinel errors.
package errs

import (
	"errors"
	"fmt"
	"regexp"
)

// Code is a stable user-facing error identifier of the form `AAI-NNN`.
type Code string

// String returns the code as a plain string.
func (c Code) String() string { return string(c) }

// Stable codes. Numbering convention:
//
//	001-099  load / parse (specs, config)
//	100-199  emit (collisions, hook conflicts)
//	200-299  import
//	300-399  sync / validate
//
// Renumbering an existing code is a breaking change; deprecate and
// add a new one instead.
const (
	// Load / parse.
	CodeSpecParse       Code = "AAI-001"
	CodeUnsupportedKind Code = "AAI-002"
	CodeConfigMissing   Code = "AAI-003"
	CodeConfigDecode    Code = "AAI-004"

	// Emit.
	CodeOutputCollision Code = "AAI-102"

	// Import.
	CodeImportFileUnknown Code = "AAI-202"

	// Sync / validate.
	CodeSyncTargetUnknown Code = "AAI-301"
	CodeFlagConflict      Code = "AAI-302"
)

// codedError is the concrete error type produced by Coded. It exposes
// the Code and an unwrap chain so callers can still match wrapped
// sentinel errors.
type codedError struct {
	code Code
	msg  string
	wrap error
}

func (e *codedError) Error() string { return "[" + string(e.code) + "] " + e.msg }
func (e *codedError) Unwrap() error { return e.wrap }
func (e *codedError) Code() Code    { return e.code }

// Coded returns an error tagged with the given code. The format/args
// follow fmt.Errorf semantics: pass `%w` to wrap an underlying error
// so errors.Is / errors.As keep finding it. The wrapped error is also
// surfaced via Unwrap; the visible message is `[AAI-NNN] <text>`.
func Coded(code Code, format string, args ...any) error {
	wrapped := fmt.Errorf(format, args...)
	return &codedError{
		code: code,
		msg:  wrapped.Error(),
		wrap: errors.Unwrap(wrapped),
	}
}

// CodeOf returns the Code attached to err, or "" if none is in the
// chain. Walks the entire chain via errors.As.
func CodeOf(err error) Code {
	var c *codedError
	if errors.As(err, &c) {
		return c.code
	}
	return ""
}

// reCode validates the AAI-NNN shape used by the explain command and
// public docs.
var reCode = regexp.MustCompile(`^AAI-\d{3,}$`)

// IsCode reports whether s syntactically matches the `AAI-NNN` shape.
// The check is shape-only; lookups still go through Lookup.
func IsCode(s string) bool { return reCode.MatchString(s) }
