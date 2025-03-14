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
	"errors"
	"log"
	"log/slog"
	"os"
	"strings"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	wccs "github.com/opencloud-eu/woodpecker-ci-config-service"
)

var (
	// cfgFile file path.
	cfgFile string
	// global configuration.
	cfg struct {
		// the log level for the service.
		LogLevel slog.Level `mapstructure:"log_level"`
		// server related configuration.
		Server serverConfiguration
		// convert related configuration.
		Convert convertConfiguration
	}
	// default logger.
	logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))
)

func init() {
	cobra.OnInitialize(onInitialize)

	viper.SetDefault("log_level", slog.LevelError)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.wccs.yaml)")
}

func Execute() error {
	return rootCmd.Execute()
}

func onInitialize() {
	switch {
	case cfgFile != "":
		viper.SetConfigFile(cfgFile)
	default:
		viper.AddConfigPath(wccs.Must1(os.UserHomeDir()))
		viper.SetConfigType("toml")
		viper.SetConfigName(".wccs")
	}

	switch err := viper.ReadInConfig(); {
	case errors.As(err, &viper.ConfigFileNotFoundError{}):
		break
	case err != nil:
		log.Fatal(err) //nolint:forbidigo
	}

	viper.SetEnvPrefix("WCCS")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	wccs.Must(viper.Unmarshal(&cfg, viper.DecodeHook(mapstructure.TextUnmarshallerHookFunc())))
}
