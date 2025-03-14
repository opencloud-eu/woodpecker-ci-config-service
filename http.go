// Copyright 2025 OpenCloud GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package wccs

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/yaronf/httpsign"
)

// ConfigurationHandler is a http handler
// that fetches the configuration files for the given repository.
func ConfigurationHandler(logger *slog.Logger, converters Converters, providers Providers) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env Environment
		if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
			logger.Error(err.Error())
			http.Error(w, "Failed to decode request", http.StatusBadRequest)
			return
		}

		logger.Debug(fmt.Sprintf("Start configuration service for %s", env.Repo.Name))

		providedFiles, err := providers.Get(r.Context(), env)
		if err != nil {
			logger.Error(err.Error())
			http.Error(w, "Failed to get config", http.StatusInternalServerError)
			return
		}

		configurationFiles, err := converters.Convert(providedFiles, env)
		if err != nil {
			logger.Error(err.Error())
			http.Error(w, "Failed to get convert", http.StatusInternalServerError)
			return
		}

		// there is no guarantee that any of the available providers will return a configuration
		// woodpecker by default expects a 204 response in this case to fall back to the repository woodpecker configurations
		if len(configurationFiles) == 0 {
			logger.Debug(fmt.Sprintf("No configurations found for %s, woodpecker takes over", env.Repo.Name))
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// cleanup configuration names
		for i, file := range configurationFiles {
			file.Name = strings.ReplaceAll(file.Name, "/", "__")
			file.Name = strings.TrimSuffix(file.Name, filepath.Ext(file.Name))
			configurationFiles[i] = file
		}

		if err := json.NewEncoder(w).Encode(map[string]any{"configs": configurationFiles}); err != nil {
			logger.Error(err.Error())
			return
		}
		logger.Debug(fmt.Sprintf("successfully fetched configurations for %s, start pipeline", env.Repo.Name))
	})
}

// VerifierMiddlewareFactory is a middleware that verifies the given request signature.
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
	if err != nil {
		return nil, err
	}

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

// AllowedMethodsMiddlewareFactory is a middleware that checks if the given request method is allowed.
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
