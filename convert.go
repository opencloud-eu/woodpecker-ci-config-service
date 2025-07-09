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
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"

	"github.com/samber/lo"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"
	"gopkg.in/yaml.v3"
)

// Converters contains multiple converters.
type Converters []Converter

// Convert converts multiple files using the available converters.
func (converters Converters) Convert(files []File, env Environment) ([]File, error) {
	var results []File
	for _, file := range files {
		for _, converter := range converters {
			if !converter.Compatible(file) {
				continue
			}

			converted, err := converter.Convert(file, env)
			if err != nil {
				return nil, err
			}

			results = append(results, converted...)
			break // only one converter should be used
		}
	}

	if duplicates := lo.FindDuplicatesBy(results, func(result File) string {
		return result.Name
	}); len(duplicates) != 0 {
		return nil, fmt.Errorf("conversion contains with dublicated files: %v", lo.Map(duplicates, func(f File, _ int) string {
			return f.Name
		}))
	}

	return results, nil
}

// StarlarkConverter is a converter that reads, transpiles and migrates Starlark configuration files.
type StarlarkConverter struct {
	logger *slog.Logger
}

// NewStarlarkConverter returns a new StarlarkConverter.
func NewStarlarkConverter(logger *slog.Logger) (StarlarkConverter, error) {
	return StarlarkConverter{logger: logger}, nil
}

func (p StarlarkConverter) Compatible(f File) bool {
	return slices.Contains([]string{".star"}, filepath.Ext(f.Name))
}

// Convert reads, transpiles and migrates Starlark configuration files to the required format.
func (p StarlarkConverter) Convert(f File, env Environment) ([]File, error) {
	if f.Data == "" {
		return nil, ErrNoContent
	}

	thread := &starlark.Thread{
		Name: "drone",
		Print: func(_ *starlark.Thread, msg string) {
			p.logger.Debug(msg)
		},
	}

	globals, err := starlark.ExecFileOptions(syntax.LegacyFileOptions(), thread, "", f.Data, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: error executing file", err)
	}

	entrypoint, ok := globals["main"]
	if !ok {
		return nil, fmt.Errorf("%w: main", ErrNoEntrypoint)
	}

	v, err := starlark.Call(thread, entrypoint, []starlark.Value{
		starlarkstruct.FromStringDict(
			starlark.String("context"),
			starlark.StringDict{
				//IMPORTANT: just a hint, never add any env.Netrc values to the context, this contains sensitive information!!!
				"repo": starlarkstruct.FromStringDict(starlark.String("repo"), starlark.StringDict{
					"owner":    starlark.String(env.Repo.Owner),
					"name":     starlark.String(env.Repo.Name),
					"fullName": starlark.String(env.Repo.FullName),
					"branch":   starlark.String(env.Repo.Branch),
				}),
				"build": starlarkstruct.FromStringDict(starlark.String("build"), starlark.StringDict{
					"event":   starlark.String(env.Pipeline.Event),
					"title":   starlark.String(env.Pipeline.Title),
					"commit":  starlark.String(env.Pipeline.Commit),
					"ref":     starlark.String(env.Pipeline.Ref),
					"branch":  starlark.String(env.Pipeline.Branch),
					"message": starlark.String(env.Pipeline.Message),
					"sender":  starlark.String(env.Pipeline.Sender),
				}),
			},
		),
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: error building conf", err)
	}

	// toDo: shame on me....
	hacky := v.String()
	hacky = strings.ReplaceAll(hacky, "False", "false")
	hacky = strings.ReplaceAll(hacky, "True", "true")
	hacky = strings.ReplaceAll(hacky, "None", "[]")

	var workflows []map[string]any
	if err := json.Unmarshal([]byte(hacky), &workflows); err != nil {
		return nil, err
	}

	var files []File
	for _, workflow := range workflows {
		name, ok := workflow["name"].(string)
		if !ok || name == "" {
			return nil, fmt.Errorf("%w: name", ErrMissingParam)
		}
		delete(workflow, "name")

		buf := new(bytes.Buffer)
		enc := yaml.NewEncoder(buf)
		enc.SetIndent(2) //nolint:mnd
		if err := enc.Encode(workflow); err != nil {
			return nil, err
		}
		files = append(files, File{
			Name: strings.TrimSuffix(name, filepath.Ext(name)) + ".yaml",
			Data: buf.String(),
		})
	}

	return files, nil
}
