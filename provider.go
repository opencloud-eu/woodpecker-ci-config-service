package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
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

// StarlarkProvider is a provider that reads, transpiles and migrates Starlark configuration files.
type StarlarkProvider struct {
	log *slog.Logger
}

// NewStarlarkLocalProvider returns a new StarlarkProvider.
func NewStarlarkLocalProvider(log *slog.Logger) StarlarkProvider {
	return StarlarkProvider{log: log}
}

// Get reads, transpiles and migrates Starlark configuration files to the required format.
func (p StarlarkProvider) Get(ce ConfigurationEnvironment) ([]Configuration, error) {
	thread := &starlark.Thread{
		Name: "drone",
		Print: func(_ *starlark.Thread, msg string) {
			p.log.Info(msg)
		},
	}

	globals, err := starlark.ExecFileOptions(syntax.LegacyFileOptions(), thread, ce.Repo.Config, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("error executing file: %v", err)
	}

	v, err := starlark.Call(thread, globals["main"], []starlark.Value{
		starlarkstruct.FromStringDict(
			starlark.String("context"),
			starlark.StringDict{
				"repo": starlarkstruct.FromStringDict(starlark.String("repo"), starlark.StringDict{
					"name": starlark.String("name"),
					"slug": starlark.String("slug"),
				}),
				"build": starlarkstruct.FromStringDict(starlark.String("build"), starlark.StringDict{
					"event":       starlark.String("event"),
					"title":       starlark.String("title"),
					"commit":      starlark.String("commit"),
					"ref":         starlark.String("ref"),
					"target":      starlark.String("target"),
					"source":      starlark.String("source"),
					"source_repo": starlark.String("source_repo"),
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

	var workflows []interface{}
	if err := json.Unmarshal([]byte(hacky), &workflows); err != nil {
		return nil, err
	}

	var configurations []Configuration
	for _, workflow := range workflows {
		buf := new(bytes.Buffer)
		enc := yaml.NewEncoder(buf)
		enc.SetIndent(2)
		if err := enc.Encode(workflow); err != nil {
			return nil, err
		}

		var transport struct {
			Name string
		}
		if err := yaml.Unmarshal(buf.Bytes(), &transport); err != nil {
			return nil, fmt.Errorf("error unmarshaling YAML: %v", err)
		}

		configurations = append(configurations, Configuration{Name: transport.Name, Data: buf.String()})
	}

	return configurations, nil
}

// FSProvider is a provider that reads configuration files from the file system.
type FSProvider struct {
	fs         fs.FS
	extensions []string
}

// NewFSProvider returns a new FSProvider.
func NewFSProvider(fsys fs.FS, extensions []string) FSProvider {
	return FSProvider{
		fs:         fsys,
		extensions: lo.Uniq(append([]string{".yml", ".yaml"}, extensions...)),
	}
}

// Get reads configuration files from the file system.
func (p FSProvider) Get(ce ConfigurationEnvironment) ([]Configuration, error) {
	// toDo: this is a hack which will go away once the new configuration can be build with Starlark
	// it should skip the provider if the pipeline path is not set to PROVIDER_FS_MIGRATION_PROCESS
	if ce.Repo.Config != "PROVIDER_FS_MIGRATION_PROCESS" {
		return nil, nil
	}

	var configurations []Configuration
	if err := fs.WalkDir(p.fs, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// check if the file is a supported file, otherwise skip
		if d.IsDir() || !slices.Contains(p.extensions, filepath.Ext(d.Name())) {
			return nil
		}

		f, err := p.fs.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()

		buf := new(bytes.Buffer)
		if _, err := buf.ReadFrom(f); err != nil {
			return err
		}

		configurations = append(configurations, Configuration{
			Name: strings.TrimSuffix(d.Name(), filepath.Ext(d.Name())),
			Data: buf.String(),
		})

		return nil
	}); err != nil {
		return nil, err
	}

	return configurations, nil
}
