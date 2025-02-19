package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/x509"
	"embed"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	libenv "github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
	"github.com/justinas/alice"
	"github.com/yaronf/httpsign"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"
	"gopkg.in/yaml.v3"
)

var (
	env struct {
		Host           string   `env:"CONFIG_SERVICE_HOST" envDefault:":8080"`
		PublicKey      string   `env:"CONFIG_SERVICE_PUBLIC_KEY"`
		ConfigDir      string   `env:"CONFIG_SERVICE_CONFIG_DIR"`
		LegacyConf     string   `env:"CONFIG_SERVICE_LEGACY_CONF"`
		LegacyDir      string   `env:"CONFIG_SERVICE_LEGACY_DIR"`
		AllowedMethods []string `env:"CONFIG_SERVICE_ALLOWED_METHODS" envDefault:"POST"`
	}

	//go:embed testenv/conf/clean/*
	embeddedConfigFS embed.FS
)

type config struct {
	Name string `json:"name"`
	Data string `json:"data"`
}

func init() {
	// load environment variables from .envrc file
	switch err := godotenv.Overload(".envrc", ".env"); {
	// it's fine if the file does not exist, maybe the environment variables are already set... who knows
	case errors.Is(err, os.ErrNotExist):
		break
	case err != nil:
		must(fmt.Errorf("error loading .env file: %v", err))
	}

	must(libenv.Parse(&env))

	// build and persist star configurations...
	// used for a step to step migration to the new configuration format
	if env.LegacyConf != "" && env.LegacyDir != "" {
		for _, config := range must1(transpileStarConfigs(env.LegacyConf)) {
			must(writeWorkflow(filepath.Join(env.LegacyDir, config.Name+".yaml"), []byte(config.Data)))
		}
	}
}

func transpileStarConfigs(fileName string) ([]config, error) {
	thread := &starlark.Thread{
		Name: "drone",
		Print: func(_ *starlark.Thread, msg string) {
			slog.Info(msg)
		},
	}

	globals, err := starlark.ExecFileOptions(syntax.LegacyFileOptions(), thread, fileName, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("error executing file: %v", err)
	}

	v, err := starlark.Call(thread, globals["main"], []starlark.Value{
		starlarkstruct.FromStringDict(
			starlark.String("context"),
			starlark.StringDict{
				"repo": starlarkstruct.FromStringDict(starlark.String("repo"), starlark.StringDict{
					"name": starlark.String("name"),
					"slug": starlark.String("slug"),
				}),
				"build": starlarkstruct.FromStringDict(starlark.String("build"), starlark.StringDict{
					"event":       starlark.String("event"),
					"title":       starlark.String("title"),
					"commit":      starlark.String("commit"),
					"ref":         starlark.String("ref"),
					"target":      starlark.String("target"),
					"source":      starlark.String("source"),
					"source_repo": starlark.String("source_repo"),
				}),
			},
		),
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("error building conf: %v", err)
	}

	// shame on me....
	hacky := v.String()
	hacky = strings.ReplaceAll(hacky, "False", "false")
	hacky = strings.ReplaceAll(hacky, "True", "true")
	hacky = strings.ReplaceAll(hacky, "None", "[]")

	var workflows []interface{}
	if err := json.Unmarshal([]byte(hacky), &workflows); err != nil {
		return nil, err
	}

	var configs []config
	for _, workflow := range workflows {
		var wfb strings.Builder
		enc := yaml.NewEncoder(&wfb)
		enc.SetIndent(2)
		if err := enc.Encode(workflow); err != nil {
			return nil, err
		}

		var transport struct {
			Name string
		}
		wf := wfb.String()
		if err := yaml.Unmarshal([]byte(wf), &transport); err != nil {
			return nil, fmt.Errorf("error unmarshaling YAML: %v", err)
		}

		configs = append(configs, config{Name: transport.Name, Data: wf})
	}

	return configs, nil
}

func writeWorkflow(name string, b []byte) error {
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

func readWorkflows(rfs fs.FS) ([]config, error) {
	var configs []config
	if err := fs.WalkDir(rfs, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// check if it is a supported file, otherwise skip
		if d.IsDir() || !slices.Contains([]string{".yaml", ".yml"}, filepath.Ext(d.Name())) {
			return nil
		}

		f, err := rfs.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()

		buf := new(bytes.Buffer)
		if _, err := buf.ReadFrom(f); err != nil {
			return err
		}

		configs = append(configs, config{
			Name: strings.TrimSuffix(d.Name(), filepath.Ext(d.Name())),
			Data: buf.String(),
		})

		return nil
	}); err != nil {
		return nil, err
	}

	return configs, nil
}

// verifierMiddleware is a middleware that verifies the given request signature
func verifierMiddleware(pubKeyPath string) (func(http.Handler) http.Handler, error) {
	if pubKeyPath == "" {
		return nil, fmt.Errorf("public key path is empty")
	}

	pubKeyRaw, err := os.ReadFile(pubKeyPath)
	if err != nil {
		return nil, err
	}

	pemBlock, _ := pem.Decode(pubKeyRaw)
	b, err := x509.ParsePKIXPublicKey(pemBlock.Bytes)
	if err != nil {
		return nil, err
	}

	pubKey, ok := b.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("public key is not of type ed25519")
	}

	verifier, err := httpsign.NewEd25519Verifier(pubKey,
		httpsign.NewVerifyConfig(),
		httpsign.Headers("@request-target", "content-digest"),
	)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := httpsign.VerifyRequest("woodpecker-ci-extensions", *verifier, r); err != nil {
				http.Error(w, "Invalid signature", http.StatusBadRequest)
				return
			}

			next.ServeHTTP(w, r)
		})
	}, nil
}

// allowedMethodsMiddleware is a middleware that checks if the given request method is allowed
func allowedMethodsMiddleware(methods ...string) (func(http.Handler) http.Handler, error) {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !slices.Contains(methods, r.Method) {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}

			next.ServeHTTP(w, r)
		})
	}, nil
}

// must is a helper that panics if the error is not nil
func must(err error) {
	if err != nil {
		slog.Error("", err)
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

func main() {
	var configFS fs.FS
	switch {
	case env.ConfigDir != "":
		configFS = os.DirFS(env.ConfigDir)
	default:
		configFS = embeddedConfigFS
	}

	middlewares := []alice.Constructor{
		must1(allowedMethodsMiddleware(env.AllowedMethods...)),
	}

	switch env.PublicKey {
	case "":
		slog.Warn("public key is empty, incoming requests will not be verified, be careful!")
	default:
		middlewares = append(middlewares, must1(verifierMiddleware(env.PublicKey)))
	}

	http.Handle("/ciconfig", alice.New(middlewares...).ThenFunc(func(w http.ResponseWriter, r *http.Request) {
		configs, err := readWorkflows(configFS)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		if err = json.NewEncoder(w).Encode(map[string]interface{}{"configs": configs}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))

	must(http.ListenAndServe(env.Host, http.DefaultServeMux))
}
