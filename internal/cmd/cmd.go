package cmd

import (
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	wcs "github.com/opencloud-eu/woodpecker-ci-config-service"
)

var (
	// cfgFile file path
	cfgFile string
	// global configuration
	cfg struct {
		// the log level for the service
		LogLevel slog.Level `mapstructure:"log_level"`
		// server related configuration
		Server struct {
			// which host to listen on
			Address string
			// the public key to verify incoming requests
			PublicKey string `mapstructure:"public_key"`
			// the providers which are used to get the configuration files
			Providers []wcs.ProviderType
			// the file system source for the fs provider
			ProviderFSSource string `mapstructure:"provider_fs_source"`
			// the endpoint to listen on
			ConfigEndpoint string `mapstructure:"config_endpoint"`
			// the allowed methods which are allowed for the config service
			ConfigEndpointMethods []string `mapstructure:"config_endpoint_methods"`
		}
	}
)

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.wcs.yaml)")

	rootCmd.AddCommand(serverCmd)
}

func initConfig() {
	viper.SetDefault("log_level", slog.LevelError)
	viper.SetDefault("server.address", ":8080")
	viper.SetDefault("server.public_key", "")
	viper.SetDefault("server.providers", []wcs.ProviderType{wcs.ProviderTypeForge})
	viper.SetDefault("server.provider_fs_source", "")
	viper.SetDefault("server.config_endpoint", "/ciconfig")
	viper.SetDefault("server.config_endpoint_methods", []string{http.MethodPost})

	switch {
	case cfgFile != "":
		viper.SetConfigFile(cfgFile)
	default:
		viper.AddConfigPath(wcs.Must1(os.UserHomeDir()))
		viper.SetConfigType("toml")
		viper.SetConfigName(".wcs")
	}
	wcs.Must(viper.ReadInConfig())

	viper.SetEnvPrefix("WCS")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	wcs.Must(viper.Unmarshal(&cfg, viper.DecodeHook(mapstructure.TextUnmarshallerHookFunc())))
}
