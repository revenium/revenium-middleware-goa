package revenium

import "fmt"

// ErrorType classifies the category of a ReveniumError.
type ErrorType string

const (
	// ErrorTypeConfig indicates a configuration error.
	ErrorTypeConfig ErrorType = "config"
	// ErrorTypeMetering indicates a metering API error.
	ErrorTypeMetering ErrorType = "metering"
	// ErrorTypeNetwork indicates a network/transport error.
	ErrorTypeNetwork ErrorType = "network"
	// ErrorTypeValidation indicates a validation error.
	ErrorTypeValidation ErrorType = "validation"
)

// ReveniumError is a typed error returned by the revenium package.
type ReveniumError struct {
	Type    ErrorType
	Message string
	Err     error
}

func (e *ReveniumError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("revenium %s error: %s: %v", e.Type, e.Message, e.Err)
	}
	return fmt.Sprintf("revenium %s error: %s", e.Type, e.Message)
}

func (e *ReveniumError) Unwrap() error {
	return e.Err
}

func newConfigError(msg string, err error) *ReveniumError {
	return &ReveniumError{Type: ErrorTypeConfig, Message: msg, Err: err}
}

func newMeteringError(msg string, err error) *ReveniumError {
	return &ReveniumError{Type: ErrorTypeMetering, Message: msg, Err: err}
}

func newNetworkError(msg string, err error) *ReveniumError {
	return &ReveniumError{Type: ErrorTypeNetwork, Message: msg, Err: err}
}
