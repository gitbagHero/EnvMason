package macos

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"time"
)

const (
	defaultCommandTimeout = 3 * time.Second
	maxCommandOutput      = 64 * 1024
)

type commandRunner interface {
	Run(context.Context, string, ...string) (string, error)
}

type execRunner struct {
	timeout time.Duration
}

func (r execRunner) Run(parent context.Context, name string, args ...string) (string, error) {
	timeout := r.timeout
	if timeout <= 0 {
		timeout = defaultCommandTimeout
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	var output limitedBuffer
	command := exec.CommandContext(ctx, name, args...)
	command.Stdout = &output
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		// Do not propagate command output: stderr can contain sensitive data.
		return "", errors.New("command failed")
	}
	if output.exceeded {
		return "", errors.New("command output exceeded limit")
	}
	return strings.TrimSpace(output.buffer.String()), nil
}

type limitedBuffer struct {
	buffer   bytes.Buffer
	exceeded bool
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	originalLength := len(data)
	remaining := maxCommandOutput + 1 - b.buffer.Len()
	if remaining <= 0 {
		b.exceeded = true
		return originalLength, nil
	}
	if len(data) > remaining {
		data = data[:remaining]
		b.exceeded = true
	}
	_, _ = b.buffer.Write(data)
	if b.buffer.Len() > maxCommandOutput {
		b.exceeded = true
	}
	return originalLength, nil
}
