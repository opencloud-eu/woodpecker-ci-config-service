package wcs_test

import (
	"log/slog"
)

var (
	noopLogger = slog.New(slog.DiscardHandler)
)
