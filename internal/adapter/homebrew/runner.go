package homebrew

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

const (
	commandTimeout = 30 * time.Second
	maxStdout      = 32 * 1024 * 1024
	maxStderr      = 64 * 1024
)

type commandOutput struct {
	stdout string
	stderr string
}

type commandRunner interface {
	Run(context.Context, string, []string, map[string]string) (commandOutput, error)
}

type execRunner struct{}

func (execRunner) Run(parent context.Context, executable string, args []string, overrides map[string]string) (commandOutput, error) {
	ctx, cancel := context.WithTimeout(parent, commandTimeout)
	defer cancel()

	command := exec.CommandContext(ctx, executable, args...)
	command.Env = mergedEnvironment(overrides)
	var stdout limitedBuffer
	stdout.limit = maxStdout
	var stderr limitedBuffer
	stderr.limit = maxStderr
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	result := commandOutput{stdout: strings.TrimSpace(stdout.buffer.String()), stderr: strings.TrimSpace(stderr.buffer.String())}
	if stdout.exceeded {
		return result, errors.New("command stdout exceeded limit")
	}
	if stderr.exceeded {
		return result, errors.New("command stderr exceeded limit")
	}
	if err != nil {
		return result, errors.New("command failed")
	}
	return result, nil
}

type limitedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	exceeded bool
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	originalLength := len(data)
	remaining := b.limit + 1 - b.buffer.Len()
	if remaining <= 0 {
		b.exceeded = true
		return originalLength, nil
	}
	if len(data) > remaining {
		data = data[:remaining]
		b.exceeded = true
	}
	_, _ = b.buffer.Write(data)
	if b.buffer.Len() > b.limit {
		b.exceeded = true
	}
	return originalLength, nil
}

func mergedEnvironment(overrides map[string]string) []string {
	values := make(map[string]string)
	for _, entry := range os.Environ() {
		name, value, ok := strings.Cut(entry, "=")
		if ok {
			values[name] = value
		}
	}
	for name, value := range overrides {
		values[name] = value
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]string, 0, len(names))
	for _, name := range names {
		result = append(result, name+"="+values[name])
	}
	return result
}
