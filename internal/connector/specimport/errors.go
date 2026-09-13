package specimport

import (
	"errors"

	"github.com/rebornace/baize/internal/connector/openapi"
)

// ErrInvalidSpec is returned when content cannot be recognized, converted, or validated.
var ErrInvalidSpec = openapi.ErrInvalidSpec

// ErrUnsupportedFormat is returned when import_format is not supported.
var ErrUnsupportedFormat = errors.New("unsupported_import_format")
