package main

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	libenv "github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
	"github.com/justinas/alice"
	"go.woodpecker-ci.org/woodpecker/v3/server/model"
)

var (
	// envFiles is a list of possible .env files
	envFiles = []string{".envrc", ".env"}

	// env is the configuration struct
	env struct {
		Host           string     `env:"CONFIG_SERVICE_HOST" envDefault:":8080"`
		PublicKey      string     `env:"CONFIG_SERVICE_PUBLIC_KEY"`
		ConfigDir      string     `env:"CONFIG_SERVICE_CONFIG_DIR"`
		ConfigEndpoint string     `env:"CONFIG_SERVICE_CONFIG_ENDPOINT" envDefault:"/ciconfig"`
		LegacyConf     string     `env:"CONFIG_SERVICE_LEGACY_CONFIG"`
		LegacyDir      string     `env:"CONFIG_SERVICE_LEGACY_DIR"`
		AllowedMethods []string   `env:"CONFIG_SERVICE_ALLOWED_METHODS" envDefault:"POST"`
		LogLevel       slog.Level `env:"CONFIG_SERVICE_LOG_LEVEL" envDefault:"error"`
	}

	// embeddedConfigFS contains the embedded configuration files
	//go:embed testenv/conf/clean/*
	embeddedConfigFS embed.FS

	log *slog.Logger
)

type (
	ConfigurationEnvironment struct {
		Repo     model.Repo     `json:"repo"`
		Pipeline model.Pipeline `json:"pipeline"`
		Netrc    model.Netrc    `json:"netrc"`
	}

	ConfigurationProvider interface {
		Get(ConfigurationEnvironment) ([]Configuration, error)
	}

	Configuration struct {
		Name string `json:"name"`
		Data string `json:"data"`
	}
)

func init() {
	// load environment variables from .envrc file
	switch err := godotenv.Overload(envFiles...); {
	// it's fine if the file does not exist, maybe the environment variables are already set... who knows
	case errors.Is(err, os.ErrNotExist):
		break
	case err != nil:
		must(fmt.Errorf("error loading .env file: %v", err))
	}

	must(libenv.Parse(&env))

	log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: env.LogLevel,
	}))

	// toDo: fake it till you make it, right ... ;)
	// the starlark provider is only used locally at the moment
	// till the new configuration format is fully implemented
	starlarkProvider := NewStarlarkLocalProvider(log)

	// build and persist star configurations...
	// used for a step to step migration to the new configuration format
	if env.LegacyConf != "" && env.LegacyDir != "" {
		for _, c := range must1(starlarkProvider.Get(ConfigurationEnvironment{Repo: model.Repo{Config: env.LegacyConf}})) {
			must(persist(filepath.Join(env.LegacyDir, c.Name+".yaml"), []byte(c.Data)))
		}
	}
}

func main() {
	var configFS fs.FS
	switch {
	case env.ConfigDir != "":
		configFS = os.DirFS(env.ConfigDir)
	default:
		configFS = embeddedConfigFS
	}

	providers := []ConfigurationProvider{
		NewFSProvider(configFS, []string{}),
	}

	middlewares := []alice.Constructor{
		must1(allowedMethodsMiddlewareFactory(env.AllowedMethods...)),
	}

	switch env.PublicKey {
	case "":
		log.Warn("public key is empty, incoming requests will not be verified, be careful!")
	default:
		middlewares = append(middlewares, must1(verifierMiddlewareFactory(env.PublicKey)))
	}

	http.Handle(env.ConfigEndpoint, alice.New(middlewares...).Then(configurationHandler(providers)))

	must(http.ListenAndServe(env.Host, http.DefaultServeMux))
}

// must is a helper that panics if the error is not nil
func must(err error) {
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
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
