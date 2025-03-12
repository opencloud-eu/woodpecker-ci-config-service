package cmd

import (
	"log/slog"
	"net/http"
	"os"
	"slices"

	"github.com/justinas/alice"
	"github.com/spf13/cobra"

	wcs "github.com/opencloud-eu/woodpecker-ci-config-service"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "start the configuration server",
	Run: func(cmd *cobra.Command, args []string) {
		logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: cfg.LogLevel,
		}))
		middlewares := []alice.Constructor{
			wcs.Must1(wcs.AllowedMethodsMiddlewareFactory(cfg.Server.ConfigEndpointMethods...)),
		}

		var providers []wcs.Provider
		if slices.Contains(cfg.Server.Providers, wcs.ProviderTypeForge) {
			providers = append(providers, wcs.Must1(wcs.NewForgeProvider(logger)))
		}

		if slices.Contains(cfg.Server.Providers, wcs.ProviderTypeFS) {
			providers = append(providers, wcs.Must1(wcs.NewFSProvider(cfg.Server.ProviderFSSource, logger)))
		}

		converters := []wcs.Converter{
			wcs.Must1(wcs.NewStarlarkConverter(logger)),
		}

		switch cfg.Server.PublicKey {
		case "":
			logger.Warn("public key is empty, incoming requests will not be verified, be careful!")
		default:
			middlewares = append(middlewares, wcs.Must1(wcs.VerifierMiddlewareFactory(cfg.Server.PublicKey)))
		}

		http.Handle(cfg.Server.ConfigEndpoint, alice.New(middlewares...).Then(wcs.ConfigurationHandler(logger, converters, providers)))

		logger.Info("listening on", "address", cfg.Server.Address)
		wcs.Must(http.ListenAndServe(cfg.Server.Address, http.DefaultServeMux))
	},
}
