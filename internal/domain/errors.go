package domain

import "fmt"

type ErrorCode string

const (
	ErrValidation   ErrorCode = "validation_error"
	ErrNotFound     ErrorCode = "not_found"
	ErrConflict     ErrorCode = "conflict"
	ErrState        ErrorCode = "invalid_state"
	ErrForbidden    ErrorCode = "forbidden"
	ErrStaleVersion ErrorCode = "stale_version"
	ErrIntegrity    ErrorCode = "integrity_error"
)

type DomainError struct {
	Code    ErrorCode
	Message string
	Field   string
}

type ItemFieldError struct {
	Index      int    `json:"index"`
	ItemNumber int    `json:"itemNumber"`
	Field      string `json:"field"`
	Message    string `json:"message"`
}

type BatchValidationError struct{ Items []ItemFieldError }

func (e *BatchValidationError) Error() string { return "批量计划校验失败" }

func (e *DomainError) Error() string { return e.Message }

func NewError(code ErrorCode, message string) error {
	return &DomainError{Code: code, Message: message}
}

func FieldError(field, message string) error {
	return &DomainError{Code: ErrValidation, Field: field, Message: message}
}

func WrapConflict(format string, args ...any) error {
	return &DomainError{Code: ErrConflict, Message: fmt.Sprintf(format, args...)}
}
