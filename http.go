package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"slices"

	"github.com/samber/lo"
	"github.com/yaronf/httpsign"
)

func configurationHandler(providers []ConfigurationProvider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ce ConfigurationEnvironment
		if err := json.NewDecoder(r.Body).Decode(&ce); err != nil {
			slog.Error(err.Error())
			http.Error(w, "Failed to decode request", http.StatusBadRequest)
			return
		}

		var configurations = make(map[string]Configuration)
		for _, provider := range providers {
			configs, err := provider.Get(ce)
			if err != nil {
				slog.Error(err.Error())
				http.Error(w, "Failed to get configs", http.StatusInternalServerError)
				return
			}

			for _, config := range configs {
				configurations[config.Name] = config
			}
		}

		configs := lo.MapToSlice(configurations, func(_ string, c Configuration) Configuration {
			return c
		})

		// there is no guarantee that any of the available providers will return a configuration
		// woodpecker by default expects a 204 response in this case to fall back to the next provider
		if len(configs) == 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if err := json.NewEncoder(w).Encode(map[string]interface{}{"configs": configs}); err != nil {
			slog.Error(err.Error())
			http.Error(w, "Failed to render configs", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	})
}

// verifierMiddleware is a middleware that verifies the given request signature
func verifierMiddlewareFactory(pubKeyPath string) (func(http.Handler) http.Handler, error) {
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
func allowedMethodsMiddlewareFactory(methods ...string) (func(http.Handler) http.Handler, error) {
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
