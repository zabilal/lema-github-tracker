package errors

import (
    "fmt"
    "net/http"
)

// ErrorType represents the type of error
type ErrorType string

const (
    // ErrorTypeValidation indicates a validation error
    ErrorTypeValidation ErrorType = "validation_error"
    // ErrorTypeNotFound indicates a not found error
    ErrorTypeNotFound ErrorType = "not_found"
    // ErrorTypeRateLimit indicates a rate limit error
    ErrorTypeRateLimit ErrorType = "rate_limit"
    // ErrorTypeServer indicates an internal server error
    ErrorTypeServer ErrorType = "server_error"
    // ErrorTypeConflict indicates a conflict error
    ErrorTypeConflict ErrorType = "conflict"
    // ErrorTypeUnauthorized indicates an unauthorized error
    ErrorTypeUnauthorized ErrorType = "unauthorized"
    // ErrorTypeForbidden indicates a forbidden error
    ErrorTypeForbidden ErrorType = "forbidden"
)

// Error represents a structured application error
type Error struct {
    Type     ErrorType   `json:"type"`
    Message  string      `json:"message"`
    Details  interface{} `json:"details,omitempty"`
    Wrapped  error       `json:"-"`
    HTTPCode int         `json:"-"`
}

// Error implements the error interface
func (e *Error) Error() string {
    if e.Wrapped != nil {
        return fmt.Sprintf("%s: %v", e.Message, e.Wrapped)
    }
    return e.Message
}

// Unwrap returns the wrapped error
func (e *Error) Unwrap() error {
    return e.Wrapped
}

// WithDetails adds details to the error
func (e *Error) WithDetails(details interface{}) *Error {
    e.Details = details
    return e
}

// New creates a new error
func New(typ ErrorType, message string) *Error {
    code := http.StatusInternalServerError
    switch typ {
    case ErrorTypeValidation:
        code = http.StatusBadRequest
    case ErrorTypeNotFound:
        code = http.StatusNotFound
    case ErrorTypeRateLimit:
        code = http.StatusTooManyRequests
    case ErrorTypeUnauthorized:
        code = http.StatusUnauthorized
    case ErrorTypeForbidden:
        code = http.StatusForbidden
    case ErrorTypeConflict:
        code = http.StatusConflict
    }
    return &Error{
        Type:     typ,
        Message:  message,
        HTTPCode: code,
    }
}

// Wrap wraps an existing error with additional context
func Wrap(err error, typ ErrorType, message string) *Error {
    return &Error{
        Type:     typ,
        Message:  message,
        Wrapped:  err,
        HTTPCode: http.StatusInternalServerError,
    }
}

// IsType checks if the error is of a specific type
func IsType(err error, typ ErrorType) bool {
    if err == nil {
        return false
    }
    if e, ok := err.(*Error); ok {
        return e.Type == typ
    }
    return false
}