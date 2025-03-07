package wcs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"
	"gopkg.in/yaml.v3"
)

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
			p.logger.Info(msg)
		},
	}

	globals, err := starlark.ExecFileOptions(syntax.LegacyFileOptions(), thread, "", f.Data, nil)
	if err != nil {
		return nil, fmt.Errorf("error executing file: %v", err)
	}

	entrypoint, ok := globals["main"]
	if !ok {
		return nil, fmt.Errorf("%w: main", ErrNoEntrypoint)
	}

	v, err := starlark.Call(thread, entrypoint, []starlark.Value{
		starlarkstruct.FromStringDict(
			starlark.String("context"),
			starlark.StringDict{
				"repo": starlarkstruct.FromStringDict(starlark.String("repo"), starlark.StringDict{
					"name": starlark.String(env.Repo.Name),
				}),
				"build": starlarkstruct.FromStringDict(starlark.String("build"), starlark.StringDict{
					"event":  starlark.String(env.Pipeline.Event),
					"title":  starlark.String(env.Pipeline.Title),
					"commit": starlark.String(env.Pipeline.Commit),
					"ref":    starlark.String(env.Pipeline.Ref),
				}),
			},
		),
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("error building conf: %v", err)
	}

	// toDo: shame on me....
	hacky := v.String()
	hacky = strings.ReplaceAll(hacky, "False", "false")
	hacky = strings.ReplaceAll(hacky, "True", "true")
	hacky = strings.ReplaceAll(hacky, "None", "[]")

	var workflows []map[string]interface{}
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
		enc.SetIndent(2)
		if err := enc.Encode(workflow); err != nil {
			return nil, err
		}

		files = append(files, File{
			Name: name,
			Data: buf.String(),
		})
	}

	return files, nil
}
