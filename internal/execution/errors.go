package execution

import "fmt"

type Code string

const (
	CodePlanInvalid          Code = "PLAN_INVALID"
	CodePlanExpired          Code = "PLAN_EXPIRED"
	CodeConfirmationRequired Code = "CONFIRMATION_REQUIRED"
	CodeActionUnregistered   Code = "ACTION_UNREGISTERED"
	CodePreconditionFailed   Code = "PRECONDITION_FAILED"
	CodeStartFailed          Code = "EXEC_START_FAILED"
	CodeExitNonZero          Code = "EXEC_EXIT_NONZERO"
	CodeAbnormalExit         Code = "EXEC_ABNORMAL_EXIT"
	CodeTimeout              Code = "EXEC_TIMEOUT"
	CodeCancelled            Code = "EXEC_CANCELLED"
	CodeInterrupted          Code = "EXEC_INTERRUPTED"
	CodeVerificationFailed   Code = "VERIFY_FAILED"
	CodeLogWriteFailed       Code = "LOG_WRITE_FAILED"
)

type ExecutionError struct {
	Code    Code
	Message string
}

func (e *ExecutionError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func executionError(code Code, message string) *ExecutionError {
	return &ExecutionError{Code: code, Message: message}
}
