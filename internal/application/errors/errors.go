package errors

import "fmt"

type Code string

const (
	CodeValidation          Code = "validation_error"
	CodeNotFound            Code = "not_found"
	CodeForbidden           Code = "forbidden"
	CodeUnauthenticated     Code = "unauthenticated"
	CodeConflict            Code = "conflict"
	CodeIdempotencyConflict Code = "idempotency_conflict"
	CodeStaleResource       Code = "stale_resource"
	CodeInvalidState        Code = "invalid_state_transition"
	CodeExternalDependency  Code = "external_dependency_unavailable"
	CodeNotImplemented      Code = "not_implemented"
)

type Error struct {
	Code      Code
	Message   string
	Field     string
	Retryable bool
	Cause     error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Code, e.Cause)
	}
	return string(e.Code) + ": " + e.Message
}
func (e *Error) Unwrap() error             { return e.Cause }
func New(code Code, message string) *Error { return &Error{Code: code, Message: message} }
func NotImplemented() *Error               { return New(CodeNotImplemented, "application use case is not wired yet") }
