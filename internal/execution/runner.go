package execution

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
)

type OSRunner struct {
	OutputLimit int
}

func (runner OSRunner) Run(ctx context.Context, spec CommandSpec) ProcessResult {
	limit := runner.OutputLimit
	if limit <= 0 {
		limit = DefaultOutputLimit
	}
	if !filepath.IsAbs(spec.Executable) || spec.Timeout <= 0 || spec.Timeout > MaximumTimeout ||
		(spec.Directory != "" && !filepath.IsAbs(spec.Directory)) {
		return ProcessResult{Failure: executionError(CodeStartFailed, "registered action produced an invalid process specification")}
	}

	runContext, cancel := context.WithTimeout(ctx, spec.Timeout)
	defer cancel()
	stdout := newBoundedBuffer(limit)
	stderr := newBoundedBuffer(limit)
	command := exec.CommandContext(runContext, spec.Executable, spec.Args...)
	command.Env = append([]string{}, spec.Environment...)
	command.Dir = spec.Directory
	command.Stdout = stdout
	command.Stderr = stderr
	configureProcessTree(command, spec.TerminateTree)

	err := command.Run()
	result := ProcessResult{Stdout: stdout.Output(), Stderr: stderr.Output()}
	if command.ProcessState != nil {
		code := command.ProcessState.ExitCode()
		result.ExitCode = &code
	}
	if err == nil {
		return result
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		result.Failure = executionError(CodeCancelled, "process was cancelled")
		return result
	}
	if errors.Is(runContext.Err(), context.DeadlineExceeded) {
		result.Failure = executionError(CodeTimeout, "process exceeded its registered timeout")
		return result
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		if exitError.ProcessState == nil || exitError.ProcessState.ExitCode() < 0 {
			result.Failure = executionError(CodeAbnormalExit, "process exited abnormally")
		} else {
			result.Failure = executionError(CodeExitNonZero, "process returned a non-zero exit code")
		}
		return result
	}
	result.Failure = executionError(CodeStartFailed, "process could not be started")
	return result
}

type boundedBuffer struct {
	buffer    bytes.Buffer
	remaining int
	truncated bool
}

func newBoundedBuffer(limit int) *boundedBuffer { return &boundedBuffer{remaining: limit} }

func (buffer *boundedBuffer) Write(value []byte) (int, error) {
	originalLength := len(value)
	if len(value) > buffer.remaining {
		value = value[:buffer.remaining]
		buffer.truncated = true
	}
	if len(value) > 0 {
		_, _ = buffer.buffer.Write(value)
		buffer.remaining -= len(value)
	}
	return originalLength, nil
}

func (buffer *boundedBuffer) Output() CapturedOutput {
	return CapturedOutput{Text: buffer.buffer.String(), Truncated: buffer.truncated}
}

var _ interface{ Write([]byte) (int, error) } = (*boundedBuffer)(nil)
