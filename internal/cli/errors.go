package cli

import "errors"

type codedError interface {
	ExitCode() int
}

type silentError interface {
	Silent() bool
}

type commandExitError struct {
	code int
}

func (e commandExitError) Error() string {
	return "command completed with a non-zero status"
}

func (e commandExitError) ExitCode() int {
	return e.code
}

func (e commandExitError) Silent() bool {
	return true
}

func newSilentExitError(code int) error {
	return commandExitError{code: code}
}

func ExitCode(err error) int {
	var coded codedError
	if errors.As(err, &coded) {
		return coded.ExitCode()
	}
	return 1
}

func IsSilentError(err error) bool {
	var silent silentError
	return errors.As(err, &silent) && silent.Silent()
}
