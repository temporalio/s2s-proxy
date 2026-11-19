package failoverversion

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

type ErrorCode string
type Direction string

const (
	ErrorUnknownLane      ErrorCode = "unknown_lane"
	ErrorUnexpectedFVI    ErrorCode = "unexpected_fvi"
	ErrorOverflow         ErrorCode = "overflow"
	ErrorInvalidClusterID ErrorCode = "invalid_cluster_id"
	ErrorDecodeFailed     ErrorCode = "decode_failed"
	ErrorEncodeFailed     ErrorCode = "encode_failed"
	ErrorUnsupported      ErrorCode = "unsupported_payload"
	ErrorStatusDetail     ErrorCode = "status_detail_failed"

	LocalToRemote Direction = "local_to_remote"
	RemoteToLocal Direction = "remote_to_local"
)

type TranslationError struct {
	Code      ErrorCode
	Direction Direction
	Type      string
	FieldPath string
	cause     error
}

func (e *TranslationError) Error() string {
	if e == nil {
		return "failover version translation failed"
	}
	parts := []string{"failover version translation failed", "code=" + string(e.Code), "direction=" + string(e.Direction)}
	if e.Type != "" {
		parts = append(parts, "type="+e.Type)
	}
	if e.FieldPath != "" {
		parts = append(parts, "field="+e.FieldPath)
	}
	return strings.Join(parts, " ")
}

func (e *TranslationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func NewTranslationError(code ErrorCode, direction Direction, typeName, fieldPath string, cause error) *TranslationError {
	return &TranslationError{
		Code:      code,
		Direction: direction,
		Type:      sanitizeSchemaPath(typeName),
		FieldPath: sanitizeSchemaPath(fieldPath),
		cause:     cause,
	}
}

func WithField(err error, typeName, fieldPath string) *TranslationError {
	var translationErr *TranslationError
	if !errors.As(err, &translationErr) || translationErr == nil {
		return NewTranslationError(ErrorUnsupported, "", typeName, fieldPath, err)
	}
	clone := *translationErr
	clone.Type = sanitizeSchemaPath(typeName)
	clone.FieldPath = sanitizeSchemaPath(fieldPath)
	return &clone
}

func sanitizeSchemaPath(value string) string {
	const maxLength = 256
	value = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("._[]", r) {
			return r
		}
		return '_'
	}, value)
	if len(value) > maxLength {
		value = value[:maxLength]
	}
	return value
}

func unexpectedValueCause(kind string) error {
	return fmt.Errorf("invalid %s", kind)
}
