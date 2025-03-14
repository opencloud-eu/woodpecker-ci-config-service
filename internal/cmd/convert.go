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
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	wccs "github.com/opencloud-eu/woodpecker-ci-config-service"
)

type convertConfiguration struct {
	// defines which providers are enabled.
	Providers []wccs.ProviderType
	// provider specific configuration.
	Provider struct {
		// fs provider configuration.
		FS struct {
			// the file system source.
			Source string
		}
	}
}

var convertCmd = &cobra.Command{
	Use:   "convert",
	Short: "convert configurations",
	Args:  cobra.MatchAll(cobra.ExactArgs(1)),
	Run: func(cmd *cobra.Command, args []string) {
		envP := args[0]
		if envP == "" {
			log.Fatal("no env provided") //nolint: forbidigo
		}

		var env wccs.Environment
		wccs.Must(json.Unmarshal([]byte(
			os.ExpandEnv(
				string(
					wccs.Must1(os.ReadFile(envP)),
				),
			),
		), &env))

		var providers wccs.Providers
		if slices.Contains(cfg.Convert.Providers, wccs.ProviderTypeForge) {
			providers = append(providers, wccs.Must1(wccs.NewForgeProvider(logger)))
		}

		if slices.Contains(cfg.Convert.Providers, wccs.ProviderTypeFS) {
			providers = append(providers, wccs.Must1(wccs.NewFSProvider(cfg.Convert.Provider.FS.Source, logger)))
		}

		converters := wccs.Converters{
			wccs.Must1(wccs.NewStarlarkConverter(logger)),
		}

		providedFiles := wccs.Must1(providers.Get(cmd.Context(), env))
		configurationFiles := wccs.Must1(converters.Convert(providedFiles, env))

		out := cmd.Flag("out")
		var report func(f wccs.File) error
		switch {
		case out != nil && out.Value.String() != "":
			report = func(c wccs.File) error {
				fp := filepath.Join(out.Value.String(), c.Name)
				if err := os.MkdirAll(filepath.Dir(fp), 0o770); err != nil { //nolint: mnd
					return err
				}

				f, err := os.Create(fp)
				if err != nil {
					return err
				}

				if _, err := f.Write([]byte(c.Data)); err != nil {
					return err
				}

				return err
			}
		default:
			report = func(c wccs.File) error {
				_, err := fmt.Fprintf(os.Stdout, "\n%s\n%s\n%s\n", c.Name, strings.Repeat("=", len(c.Name)), c.Data)
				return err
			}
		}
		for _, f := range configurationFiles {
			wccs.Must(report(f))
		}
	},
}

func init() {
	viper.SetDefault("convert.providers", []wccs.ProviderType{wccs.ProviderTypeForge})
	viper.SetDefault("convert.provider.fs.source", "")

	convertCmd.Flags().String("out", "", "output directory path")

	rootCmd.AddCommand(convertCmd)
}
