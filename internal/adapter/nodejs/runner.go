package nodejs

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
	commandTimeout = 10 * time.Second
	maxOutput      = 64 * 1024
)

type commandRunner interface {
	Run(context.Context, string, []string, string, map[string]string) (string, error)
}

type execRunner struct {
	timeout     time.Duration
	outputLimit int
}

func (r execRunner) Run(parent context.Context, executable string, args []string, directory string, overrides map[string]string) (string, error) {
	timeout := r.timeout
	if timeout <= 0 {
		timeout = commandTimeout
	}
	limit := r.outputLimit
	if limit <= 0 {
		limit = maxOutput
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	command := exec.CommandContext(ctx, executable, args...)
	command.Dir = directory
	command.Env = controlledEnvironment(overrides)
	var stdout boundedBuffer
	stdout.limit = limit
	var stderr boundedBuffer
	stderr.limit = limit
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if stdout.exceeded || stderr.exceeded {
		return "", errors.New("command output exceeded limit")
	}
	if err != nil {
		return "", errors.New("command failed")
	}
	return strings.TrimSpace(stdout.buffer.String()), nil
}

type boundedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	exceeded bool
}

func (b *boundedBuffer) Write(data []byte) (int, error) {
	length := len(data)
	remaining := b.limit + 1 - b.buffer.Len()
	if remaining <= 0 {
		b.exceeded = true
		return length, nil
	}
	if len(data) > remaining {
		data = data[:remaining]
		b.exceeded = true
	}
	_, _ = b.buffer.Write(data)
	if b.buffer.Len() > b.limit {
		b.exceeded = true
	}
	return length, nil
}

func controlledEnvironment(overrides map[string]string) []string {
	values := make(map[string]string)
	for _, key := range []string{"SYSTEMROOT", "WINDIR", "COMSPEC", "PATHEXT", "TMPDIR", "TMP", "TEMP"} {
		if value, ok := os.LookupEnv(key); ok {
			values[key] = value
		}
	}
	for key, value := range overrides {
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+values[key])
	}
	return result
}
