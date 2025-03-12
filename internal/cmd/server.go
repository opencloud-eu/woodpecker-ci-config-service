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

package cmd

import (
	"log/slog"
	"net/http"
	"os"
	"slices"

	"github.com/justinas/alice"
	"github.com/spf13/cobra"

	"github.com/opencloud-eu/woodpecker-ci-config-service"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "start the configuration server",
	Run: func(_ *cobra.Command, _ []string) {
		logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: cfg.LogLevel,
		}))
		middlewares := []alice.Constructor{
			wccs.Must1(wccs.AllowedMethodsMiddlewareFactory(cfg.Server.ConfigEndpointMethods...)),
		}

		var providers []wccs.Provider
		if slices.Contains(cfg.Server.Providers, wccs.ProviderTypeForge) {
			providers = append(providers, wccs.Must1(wccs.NewForgeProvider(logger)))
		}

		if slices.Contains(cfg.Server.Providers, wccs.ProviderTypeFS) {
			providers = append(providers, wccs.Must1(wccs.NewFSProvider(cfg.Server.ProviderFSSource, logger)))
		}

		converters := []wccs.Converter{
			wccs.Must1(wccs.NewStarlarkConverter(logger)),
		}

		switch cfg.Server.PublicKey {
		case "":
			logger.Warn("public key is empty, incoming requests will not be verified, be careful!")
		default:
			middlewares = append(middlewares, wccs.Must1(wccs.VerifierMiddlewareFactory(cfg.Server.PublicKey)))
		}

		http.Handle(cfg.Server.ConfigEndpoint, alice.New(middlewares...).Then(wccs.ConfigurationHandler(logger, converters, providers)))

		logger.Info("listening on", "address", cfg.Server.Address)
		wccs.Must(http.ListenAndServe(cfg.Server.Address, http.DefaultServeMux))
	},
}
