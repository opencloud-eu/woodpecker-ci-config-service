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
	"strings"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/opencloud-eu/woodpecker-ci-config-service"
)

var (
	// cfgFile file path.
	cfgFile string
	// global configuration.
	cfg struct {
		// the log level for the service.
		LogLevel slog.Level `mapstructure:"log_level"`
		// server related configuration.
		Server struct {
			// which host to listen on.
			Address string
			// the public key to verify incoming requests.
			PublicKey string `mapstructure:"public_key"`
			// the providers which are used to get the configuration files.
			Providers []wccs.ProviderType
			// the file system source for the fs provider.
			ProviderFSSource string `mapstructure:"provider_fs_source"`
			// the endpoint to listen on.
			ConfigEndpoint string `mapstructure:"config_endpoint"`
			// the allowed methods which are allowed for the config service.
			ConfigEndpointMethods []string `mapstructure:"config_endpoint_methods"`
		}
	}
)

func Execute() error {
	cobra.OnInitialize(func() {
		viper.SetDefault("log_level", slog.LevelError)
		viper.SetDefault("server.address", ":8080")
		viper.SetDefault("server.public_key", "")
		viper.SetDefault("server.providers", []wccs.ProviderType{wccs.ProviderTypeForge})
		viper.SetDefault("server.provider_fs_source", "")
		viper.SetDefault("server.config_endpoint", "/ciconfig")
		viper.SetDefault("server.config_endpoint_methods", []string{http.MethodPost})

		switch {
		case cfgFile != "":
			viper.SetConfigFile(cfgFile)
		default:
			viper.AddConfigPath(wccs.Must1(os.UserHomeDir()))
			viper.SetConfigType("toml")
			viper.SetConfigName(".wccs")
		}
		wccs.Must(viper.ReadInConfig())

		viper.SetEnvPrefix("WCS")
		viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
		viper.AutomaticEnv()

		wccs.Must(viper.Unmarshal(&cfg, viper.DecodeHook(mapstructure.TextUnmarshallerHookFunc())))
	})

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.wccs.yaml)")

	rootCmd.AddCommand(serverCmd)

	return rootCmd.Execute()
}
