package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
	"github.com/justinas/alice"
	"go.woodpecker-ci.org/woodpecker/v3/server/model"
)

var (
	// ErrUnknownType is returned when the type is unknown
	ErrUnknownType = fmt.Errorf("unknown type")
	// ErrNoConfig is returned when no configuration file is provided
	ErrNoConfig = fmt.Errorf("no configuration file provided")
)

type (
	// CFG provides required configuration for the service
	CFG struct {
		// which host to listen on
		Host string `env:"CONFIG_SERVICE_HOST" envDefault:":8080"`
		// the endpoint to listen on
		ConfigEndpoint string `env:"CONFIG_SERVICE_CONFIG_ENDPOINT" envDefault:"/ciconfig"`
		// the allowed methods which are allowed for the config service
		AllowedMethods []string `env:"CONFIG_SERVICE_ALLOWED_METHODS" envDefault:"POST"`
		// the public key to verify incoming requests
		PublicKey string `env:"CONFIG_SERVICE_PUBLIC_KEY"`
		// the providers which are used to get the configuration files
		Providers []ProviderType `env:"CONFIG_SERVICE_PROVIDER_TYPES"`
		// the file system source for the fs provider
		ProviderFSSource string `env:"CONFIG_SERVICE_PROVIDER_FS_SOURCE"`
		// the log level for the service
		LogLevel slog.Level `env:"CONFIG_SERVICE_LOG_LEVEL" envDefault:"error"`
	}

	// Environment represents the environment for the configuration
	Environment struct {
		Repo     *model.Repo     `json:"repo"`
		Pipeline *model.Pipeline `json:"pipeline"`
		Netrc    *model.Netrc    `json:"netrc"`
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
		Data []byte `json:"data"`
	}
)

func main() {
	// load environment variables from .envrc file
	switch err := godotenv.Overload(".envrc", ".env"); {
	// it's fine if the file does not exist, maybe the environment variables are already set... who knows
	case errors.Is(err, os.ErrNotExist):
		break
	case err != nil:
		must(fmt.Errorf("error loading .env file: %v", err))
	}

	var cfg CFG
	must(env.Parse(&cfg))

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))

	middlewares := []alice.Constructor{
		must1(allowedMethodsMiddlewareFactory(cfg.AllowedMethods...)),
	}

	var providers []Provider

	if slices.Contains(cfg.Providers, ProviderTypeForge) {
		providers = append(providers, must1(NewForgeProvider(logger)))
	}

	if slices.Contains(cfg.Providers, ProviderTypeFS) {
		providers = append(providers, must1(NewFSProvider(cfg.ProviderFSSource, "*.yaml", logger)))
	}

	converters := []Converter{
		must1(NewStarlarkConverter(logger)),
	}

	switch cfg.PublicKey {
	case "":
		logger.Warn("public key is empty, incoming requests will not be verified, be careful!")
	default:
		middlewares = append(middlewares, must1(verifierMiddlewareFactory(cfg.PublicKey)))
	}

	http.Handle(cfg.ConfigEndpoint, alice.New(middlewares...).Then(configurationHandler(logger, converters, providers)))

	must(http.ListenAndServe(cfg.Host, http.DefaultServeMux))
}

// must is a helper that panics if the error is not nil
func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

// must1 is a helper that panics if the error is not nil
func must1[T any](t T, err error) T {
	if err != nil {
		must(err)
	}

	return t
}

func persist(name string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(name), 0770); err != nil {
		return err
	}

	f, err := os.Create(name)
	if err != nil {
		return err
	}

	if _, err = f.Write(b); err != nil {
		return err
	}

	return f.Close()
}
