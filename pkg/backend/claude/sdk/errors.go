package sdk

import "fmt"

// SDKError is the base error type for all SDK errors.
type SDKError struct {
	Message string
	Err     error
}

func (e *SDKError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *SDKError) Unwrap() error { return e.Err }

// CLIConnectionError indicates a failure to connect to or communicate with the CLI process.
type CLIConnectionError struct {
	SDKError
}

// CLINotFoundError indicates the claude binary was not found.
type CLINotFoundError struct {
	SDKError
	CLIPath string
}

func (e *CLINotFoundError) Error() string {
	return fmt.Sprintf("claude CLI not found at %q: %v", e.CLIPath, e.Err)
}

// ProcessError indicates the CLI process exited with an error.
type ProcessError struct {
	SDKError
	ExitCode int
	Stderr   string
}

func (e *ProcessError) Error() string {
	if e.Stderr != "" {
		return fmt.Sprintf("claude process exited with code %d: %s", e.ExitCode, e.Stderr)
	}
	return fmt.Sprintf("claude process exited with code %d", e.ExitCode)
}

// JSONDecodeError indicates a failure to parse JSON from CLI output.
type JSONDecodeError struct {
	SDKError
	Line string
}

func (e *JSONDecodeError) Error() string {
	return fmt.Sprintf("failed to decode JSON: %v (line: %s)", e.Err, e.Line)
}
