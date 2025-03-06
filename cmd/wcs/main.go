package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"slices"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
	"github.com/justinas/alice"

	"github.com/opencloud-eu/woodpecker-ci-config-service"
)

// CFG provides required configuration for the service
type CFG struct {
	// which host to listen on
	Addr string `env:"CONFIG_SERVICE_ADDRESS" envDefault:":8080"`
	// the endpoint to listen on
	ConfigEndpoint string `env:"CONFIG_SERVICE_CONFIG_ENDPOINT" envDefault:"/ciconfig"`
	// the allowed methods which are allowed for the config service
	AllowedMethods []string `env:"CONFIG_SERVICE_ALLOWED_METHODS" envDefault:"POST"`
	// the public key to verify incoming requests
	PublicKey string `env:"CONFIG_SERVICE_PUBLIC_KEY"`
	// the providers which are used to get the configuration files
	Providers []wcs.ProviderType `env:"CONFIG_SERVICE_PROVIDER_TYPES"`
	// the file system source for the fs provider
	ProviderFSSource string `env:"CONFIG_SERVICE_PROVIDER_FS_SOURCE"`
	// the log level for the service
	LogLevel slog.Level `env:"CONFIG_SERVICE_LOG_LEVEL" envDefault:"error"`
}

func main() {
	// load environment variables from .envrc file
	switch err := godotenv.Overload(".envrc", ".env"); {
	// it's fine if the file does not exist, maybe the environment variables are already set... who knows
	case errors.Is(err, os.ErrNotExist):
		break
	case err != nil:
		wcs.Must(fmt.Errorf("error loading .env file: %v", err))
	}

	var cfg CFG
	wcs.Must(env.Parse(&cfg))

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))

	middlewares := []alice.Constructor{
		wcs.Must1(wcs.AllowedMethodsMiddlewareFactory(cfg.AllowedMethods...)),
	}

	var providers []wcs.Provider

	if slices.Contains(cfg.Providers, wcs.ProviderTypeForge) {
		providers = append(providers, wcs.Must1(wcs.NewForgeProvider(logger)))
	}

	if slices.Contains(cfg.Providers, wcs.ProviderTypeFS) {
		providers = append(providers, wcs.Must1(wcs.NewFSProvider(cfg.ProviderFSSource, "*.yaml", logger)))
	}

	converters := []wcs.Converter{
		wcs.Must1(wcs.NewStarlarkConverter(logger)),
	}

	switch cfg.PublicKey {
	case "":
		logger.Warn("public key is empty, incoming requests will not be verified, be careful!")
	default:
		middlewares = append(middlewares, wcs.Must1(wcs.VerifierMiddlewareFactory(cfg.PublicKey)))
	}

	http.Handle(cfg.ConfigEndpoint, alice.New(middlewares...).Then(wcs.ConfigurationHandler(logger, converters, providers)))

	logger.Info("listening on", "address", cfg.Addr)
	wcs.Must(http.ListenAndServe(cfg.Addr, http.DefaultServeMux))
}
