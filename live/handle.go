package live

import (
	"errors"
	"net/http"
)

// InputError is a refused command input; dispatch answers it with 400.
type InputError struct {
	// Field is the offending input name ("" when it is the whole input).
	Field string `json:"field,omitempty"`
	// Message is shown to the user; keep it free of internals.
	Message string `json:"message"`
}

func (e *InputError) Error() string {
	if e.Field == "" {
		return "live: invalid input: " + e.Message
	}
	return "live: invalid input " + e.Field + ": " + e.Message
}

// Invalid returns a 400 InputError for field.
func Invalid(field, message string) error { return &InputError{Field: field, Message: message} }

// StatusError is a handler outcome answered with Code and Message as plain text.
type StatusError struct {
	// Code is the HTTP status (4xx or 5xx).
	Code int
	// Message is shown to the caller; keep it free of internals.
	Message string
}

func (e *StatusError) Error() string {
	return "live: " + http.StatusText(e.Code) + ": " + e.Message
}

// Status returns a StatusError; codes outside 400-599 answer 500.
func Status(code int, message string) error { return &StatusError{Code: code, Message: message} }

// NotFound returns a 404 StatusError.
func NotFound(message string) error { return Status(http.StatusNotFound, message) }

// HTTPStatus returns the status an error answers with: 400 for input errors, a StatusError's code, else 500.
func HTTPStatus(err error) int {
	if len(InputErrors(err)) > 0 {
		return http.StatusBadRequest
	}
	var se *StatusError
	if errors.As(err, &se) && se.Code >= 400 && se.Code <= 599 {
		return se.Code
	}
	return http.StatusInternalServerError
}

func writeStatusError(w http.ResponseWriter, err error) bool {
	var se *StatusError
	if !errors.As(err, &se) {
		return false
	}
	code := se.Code
	if code < 400 || code > 599 {
		code = http.StatusInternalServerError
	}
	http.Error(w, se.Message, code)
	return true
}

// Validator is an optional rule set that Handle runs on the decoded input.
type Validator interface{ Validate() error }

// Handle adapts a typed handler to a CommandHandler; refused input answers 400 and
// a StatusError keeps its code.
func Handle[In any](fn func(s *Session, in In) (any, error)) CommandHandler {
	return func(s *Session, args Args) (any, error) {
		in, err := decodeInput[In](args)
		if err != nil {
			return nil, err
		}
		return fn(s, in)
	}
}
