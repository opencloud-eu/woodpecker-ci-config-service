package wcs

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/samber/lo"
	"github.com/yaronf/httpsign"
)

// ConfigurationHandler is a http handler
// that fetches the configuration files for the given repository
func ConfigurationHandler(logger *slog.Logger, converters []Converter, providers []Provider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env Environment
		if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
			logger.Error(err.Error())
			http.Error(w, "Failed to decode request", http.StatusBadRequest)
			return
		}

		logger.Debug(fmt.Sprintf("Start configuration service for %s", env.Repo.Name))

		var configurations []File
		for _, provider := range providers {
			providerFiles, err := provider.Get(r.Context(), env)
			switch {
			// not having a configuration is not a critical error per se...
			case errors.Is(err, ErrNoConfig):
				fallthrough
			// not knowing the type is not a critical error per se...
			case errors.Is(err, ErrUnknownType):
				// ... ignore the error and continue
				continue
			case err != nil:
				logger.Error(err.Error())
				http.Error(w, "Failed to get config", http.StatusInternalServerError)
				return
			}

			configurations = append(configurations, providerFiles...)
		}

		for i, configuration := range configurations {
			for _, converter := range converters {
				if !converter.Compatible(configuration) {
					continue
				}

				results, err := converter.Convert(configuration, env)
				if err != nil {
					logger.Error(err.Error())
					http.Error(w, "Failed to get configs", http.StatusInternalServerError)
					return
				}

				configurations = append(configurations[:i], configurations[i+1:]...)
				configurations = append(configurations, results...)
				break // only one converter should be used
			}
		}

		// there is no guarantee that any of the available providers will return a configuration
		// woodpecker by default expects a 204 response in this case to fall back to the repository woodpecker configurations
		if len(configurations) == 0 {
			logger.Debug(fmt.Sprintf("No configurations found for %s, woodpecker takes over", env.Repo.Name))
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// cleanup configuration names
		for i, configuration := range configurations {
			configuration.Name = strings.Replace(configuration.Name, "/", "__", -1)
			configuration.Name = strings.TrimSuffix(configuration.Name, filepath.Ext(configuration.Name))
			configurations[i] = configuration
		}

		if duplicates := lo.FindDuplicatesBy(configurations, func(f File) string {
			return f.Name
		}); len(duplicates) != 0 {
			logger.Error(fmt.Sprintf("duplicate configuration files found: %v", lo.Map(duplicates, func(f File, _ int) string {
				return f.Name
			})))
			http.Error(w, "Duplicate configuration files found", http.StatusInternalServerError)
			return
		}

		logger.Debug(fmt.Sprintf("Sucessfully fetched configurations for %s, start pipeline", env.Repo.Name))
		if err := json.NewEncoder(w).Encode(map[string]interface{}{"configs": configurations}); err != nil {
			logger.Error(err.Error())
			return
		}
	})
}

// VerifierMiddlewareFactory is a middleware that verifies the given request signature
func VerifierMiddlewareFactory(pubKeyPath string) (func(http.Handler) http.Handler, error) {
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

// AllowedMethodsMiddlewareFactory is a middleware that checks if the given request method is allowed
func AllowedMethodsMiddlewareFactory(methods ...string) (func(http.Handler) http.Handler, error) {
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
