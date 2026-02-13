package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
)

// RawMessage is a single line of JSON from the CLI with optional parse error.
type RawMessage struct {
	Data json.RawMessage
	Err  error
}

// Transport abstracts communication with the Claude CLI process.
type Transport interface {
	// Connect starts the transport and the underlying process.
	Connect(ctx context.Context) error

	// Write sends a JSON-encoded message to the CLI's stdin.
	Write(data string) error

	// WriteJSON marshals v to JSON and sends it to stdin.
	WriteJSON(v any) error

	// ReadMessages returns a channel that yields raw JSON lines from stdout.
	// The channel is closed when the process exits or the context is cancelled.
	ReadMessages() <-chan RawMessage

	// Close terminates the transport and cleans up resources.
	Close() error

	// IsReady returns true if the transport is connected and ready.
	IsReady() bool

	// EndInput closes stdin, signaling no more input will be sent.
	EndInput() error

	// Wait waits for the underlying process to exit and returns any error.
	Wait() error
}

// SubprocessTransport implements Transport by spawning the Claude CLI.
type SubprocessTransport struct {
	opts Options

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	messages chan RawMessage
	ready    bool
	mu       sync.Mutex
	once     sync.Once // for cleanup
}

// NewSubprocessTransport creates a new transport that will spawn the CLI.
func NewSubprocessTransport(opts Options) *SubprocessTransport {
	return &SubprocessTransport{
		opts: opts,
	}
}

// Connect starts the CLI subprocess and begins reading stdout.
func (t *SubprocessTransport) Connect(ctx context.Context) error {
	args := []string{
		"--output-format", "stream-json",
		"--input-format", "stream-json",
		"--verbose",
	}
	args = append(args, t.opts.args()...)

	cliPath := t.opts.cliPath()
	cmd := exec.CommandContext(ctx, cliPath, args...)

	if t.opts.Cwd != "" {
		cmd.Dir = t.opts.Cwd
	}

	// Set environment
	cmd.Env = t.opts.buildEnv(os.Environ())

	// Create process group for cleanup
	cmd.SysProcAttr = processSysProcAttr()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return &CLIConnectionError{SDKError: SDKError{Message: "stdin pipe", Err: err}}
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return &CLIConnectionError{SDKError: SDKError{Message: "stdout pipe", Err: err}}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return &CLIConnectionError{SDKError: SDKError{Message: "stderr pipe", Err: err}}
	}

	if err := cmd.Start(); err != nil {
		if isNotFound(err) {
			return &CLINotFoundError{
				SDKError: SDKError{Message: "start claude", Err: err},
				CLIPath:  cliPath,
			}
		}
		return &CLIConnectionError{SDKError: SDKError{Message: "start claude", Err: err}}
	}

	t.cmd = cmd
	t.stdin = stdin
	t.stdout = stdout
	t.stderr = stderr
	t.messages = make(chan RawMessage, 64)

	t.mu.Lock()
	t.ready = true
	t.mu.Unlock()

	// Read stderr in background
	go t.readStderr()

	// Read stdout in background, sending lines to the messages channel
	go t.readStdout(ctx)

	// Handle context cancellation
	go func() {
		<-ctx.Done()
		t.Close()
	}()

	return nil
}

// Write sends raw data to stdin.
func (t *SubprocessTransport) Write(data string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stdin == nil {
		return &CLIConnectionError{SDKError: SDKError{Message: "transport not connected"}}
	}
	_, err := io.WriteString(t.stdin, data)
	return err
}

// WriteJSON marshals v and sends it followed by a newline.
func (t *SubprocessTransport) WriteJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return t.Write(string(data) + "\n")
}

// ReadMessages returns the channel of raw JSON messages from stdout.
func (t *SubprocessTransport) ReadMessages() <-chan RawMessage {
	return t.messages
}

// Close terminates the CLI process group.
func (t *SubprocessTransport) Close() error {
	t.once.Do(func() {
		t.mu.Lock()
		t.ready = false
		t.mu.Unlock()

		if t.cmd != nil && t.cmd.Process != nil {
			killProcessGroup(t.cmd.Process.Pid)
		}
	})
	return nil
}

// IsReady returns true if the transport is connected.
func (t *SubprocessTransport) IsReady() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.ready
}

// EndInput closes stdin.
func (t *SubprocessTransport) EndInput() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stdin != nil {
		return t.stdin.Close()
	}
	return nil
}

// Wait waits for the subprocess to exit.
func (t *SubprocessTransport) Wait() error {
	if t.cmd == nil {
		return nil
	}
	return t.cmd.Wait()
}

func (t *SubprocessTransport) readStdout(ctx context.Context) {
	defer close(t.messages)

	bufSize := 10 * 1024 * 1024 // 10MB default
	if t.opts.MaxBufferSize > 0 {
		bufSize = t.opts.MaxBufferSize
	}

	scanner := bufio.NewScanner(t.stdout)
	scanner.Buffer(make([]byte, 0, 1024*1024), bufSize)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		// Copy the line since scanner reuses the buffer
		cp := make([]byte, len(line))
		copy(cp, line)

		select {
		case t.messages <- RawMessage{Data: json.RawMessage(cp)}:
		case <-ctx.Done():
			return
		}
	}

	if err := scanner.Err(); err != nil {
		select {
		case t.messages <- RawMessage{Err: fmt.Errorf("scanner: %w", err)}:
		case <-ctx.Done():
		}
	}
}

func (t *SubprocessTransport) readStderr() {
	if t.opts.Stderr == nil {
		// Drain stderr to avoid blocking
		_, _ = io.Copy(io.Discard, t.stderr)
		return
	}

	scanner := bufio.NewScanner(t.stderr)
	for scanner.Scan() {
		t.opts.Stderr(scanner.Text())
	}
}

// isNotFound checks if an error is an exec.ErrNotFound.
func isNotFound(err error) bool {
	if err == exec.ErrNotFound {
		return true
	}
	if e, ok := err.(*exec.Error); ok {
		return e.Err == exec.ErrNotFound
	}
	return false
}
