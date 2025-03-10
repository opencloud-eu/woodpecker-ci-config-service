package wcs

import (
	"context"
	"fmt"
	"log"

	"go.woodpecker-ci.org/woodpecker/v3/server/model"
)

var (
	// ErrUnknownType is returned when the type is unknown
	ErrUnknownType = fmt.Errorf("unknown type")
	// ErrNoConfig is returned when no configuration file is provided
	ErrNoConfig = fmt.Errorf("no configuration file provided")
	// ErrNoContent is returned when no content is provided
	ErrNoContent = fmt.Errorf("no content provided")
	// ErrNoEntrypoint is returned when no entrypoint is found
	ErrNoEntrypoint = fmt.Errorf("no entrypoint found")
	// ErrMissingParam is returned when a parameter is missing
	ErrMissingParam = fmt.Errorf("missing parameter")
)

type (
	// Environment represents the environment for the configuration
	Environment struct {
		Repo     model.Repo     `json:"repo"`
		Pipeline model.Pipeline `json:"pipeline"`
		Netrc    model.Netrc    `json:"netrc"`
	}

	// Provider provides the configuration file
	Provider interface {
		Get(context.Context, Environment) ([]File, error)
	}

	// Converter converts the given data to a slice of files
	Converter interface {
		Convert(File, Environment) ([]File, error)
		Compatible(f File) bool
	}

	// File represents a file
	File struct {
		Name string `json:"name"`
		Data string `json:"data"`
	}
)

// Must is a helper that panics if the error is not nil
func Must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

// Must1 is a helper that panics if the error is not nil
func Must1[T any](t T, err error) T {
	if err != nil {
		Must(err)
	}

	return t
}
