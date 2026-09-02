package protocol

import "encoding/json"

// ErrorCode is a stable, redacted protocol-v1 failure classification.
type ErrorCode string

const (
	ErrorInvalidInvocation     ErrorCode = "invalid_invocation"
	ErrorBackendUnavailable    ErrorCode = "backend_unavailable"
	ErrorBackendTimeout        ErrorCode = "backend_timeout"
	ErrorOperationCanceled     ErrorCode = "operation_canceled"
	ErrorBackendInvalidData    ErrorCode = "backend_invalid_data"
	ErrorBackendOutputTooLarge ErrorCode = "backend_output_too_large"
	ErrorBackend               ErrorCode = "backend_error"
	ErrorOutput                ErrorCode = "output_error"
)

// ErrorDetail is the closed error payload. It intentionally has no message,
// details, path, command, or cause fields.
type ErrorDetail struct {
	Code ErrorCode `json:"code"`
}

// ErrorEnvelope is the closed protocol-v1 error response.
type ErrorEnvelope struct {
	Protocol int         `json:"protocol"`
	Error    ErrorDetail `json:"error"`
}

// MarshalError returns one JSON object plus LF for an allowed protocol-v1
// code. Unknown codes are rejected rather than serialized.
func MarshalError(code ErrorCode) ([]byte, bool) {
	if !validErrorCode(code) {
		return nil, false
	}
	encoded, err := json.Marshal(ErrorEnvelope{
		Protocol: ProtocolVersion,
		Error:    ErrorDetail{Code: code},
	})
	if err != nil {
		return nil, false
	}
	return append(encoded, '\n'), true
}

func validErrorCode(code ErrorCode) bool {
	switch code {
	case ErrorInvalidInvocation,
		ErrorBackendUnavailable,
		ErrorBackendTimeout,
		ErrorOperationCanceled,
		ErrorBackendInvalidData,
		ErrorBackendOutputTooLarge,
		ErrorBackend,
		ErrorOutput:
		return true
	default:
		return false
	}
}
