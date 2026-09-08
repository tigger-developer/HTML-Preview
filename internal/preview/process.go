// ABOUTME: Bounds direct converter and desktop-helper process execution.
// ABOUTME: Owns cancellation, output accounting, and native host operations.
package preview

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

// Host exposes volatile OS boundaries for focused failure and platform tests.
type Host struct {
	OS       string
	Execute  func(context.Context, Command) ([]byte, error)
	LookPath func(string) (string, error)
	Kernel   func() (string, error)
	Temp     func() (string, error)
	Remove   func(string) error
	Write    func(string, []byte) error
}

// Command is a direct executable invocation; document data never enters a shell.
type Command struct {
	Path  string
	Args  []string
	Input []byte
	Dir   string
	Limit int64
}

// NativeHost binds the command to the running system.
func NativeHost() Host {
	return Host{OS: runtime.GOOS, Execute: runProcess, LookPath: exec.LookPath,
		Kernel: func() (string, error) {
			f, err := os.Open("/proc/sys/kernel/osrelease")
			if err != nil {
				return "", err
			}
			data, err := io.ReadAll(io.LimitReader(f, 4097))
			closeErr := f.Close()
			if err != nil {
				return "", err
			}
			if closeErr != nil {
				return "", closeErr
			}
			if len(data) > 4096 {
				return "", errors.New("kernel identifier exceeds 4096 bytes")
			}
			return string(data), nil
		},
		Temp:   func() (string, error) { return os.MkdirTemp("", "htmlpreview-") },
		Remove: os.RemoveAll,
		Write:  func(path string, data []byte) error { return os.WriteFile(path, data, 0600) },
	}
}

type processOutput struct {
	mu        sync.Mutex
	remaining int64
	stdout    bytes.Buffer
	exceeded  bool
	cancel    context.CancelFunc
}
type outputWriter struct {
	output *processOutput
	keep   bool
}

func (w outputWriter) Write(p []byte) (int, error) {
	w.output.mu.Lock()
	defer w.output.mu.Unlock()
	if int64(len(p)) > w.output.remaining {
		w.output.exceeded = true
		w.output.cancel()
		return 0, errors.New("child output limit exceeded")
	}
	w.output.remaining -= int64(len(p))
	if w.keep {
		return w.output.stdout.Write(p)
	}
	return len(p), nil // Stderr consumes the budget, but source content is never logged.
}

func runProcess(ctx context.Context, request Command) ([]byte, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	output := processOutput{remaining: request.Limit, cancel: cancel}
	// #nosec G204 -- Executables are resolved during preflight; source data stays in direct argv or stdin.
	cmd := exec.CommandContext(ctx, request.Path, request.Args...)
	cmd.Dir = request.Dir
	cmd.Stdin = bytes.NewReader(request.Input)
	cmd.Stdout = outputWriter{&output, true}
	cmd.Stderr = outputWriter{&output, false}
	cmd.WaitDelay = 100 * time.Millisecond
	err := cmd.Run()
	if output.exceeded {
		return nil, errors.New("child output limit exceeded")
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, fmt.Errorf("process failed: %w", err)
	}
	return output.stdout.Bytes(), nil
}

func helper(ctx context.Context, host Host, path string, args []string, input []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return host.Execute(ctx, Command{Path: path, Args: args, Input: input, Limit: 65536})
}
