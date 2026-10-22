package eval

import "errors"

var (
	// ErrConflict is returned when a case already exists for the source run without replace.
	ErrConflict = errors.New("eval case already exists for source run")
	// ErrRunNotFound is returned when ingest targets a missing run.
	ErrRunNotFound = errors.New("run not found")
	// ErrCaseNotFound is returned when an eval case id is missing.
	ErrCaseNotFound = errors.New("eval case not found")
	// ErrNotScorable is returned when scoring a draft or incomplete case.
	ErrNotScorable = errors.New("eval case not ready to score")
	// ErrNotCalibratable is returned when a case lacks a confirmed score snapshot.
	ErrNotCalibratable = errors.New("eval case not calibratable")
	// ErrNoteTooLong is returned when human_note exceeds the rune limit after trim.
	ErrNoteTooLong = errors.New("human note too long")
	// ErrMockBaseRequired is returned when check-state is called without a mock base URL.
	ErrMockBaseRequired = errors.New("mock base url required")
	// ErrInvalidSample is returned when sample mode/template is invalid.
	ErrInvalidSample = errors.New("invalid sample request")
)
