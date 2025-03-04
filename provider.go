package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"

	"go.woodpecker-ci.org/woodpecker/v3/server/forge"
	"go.woodpecker-ci.org/woodpecker/v3/server/forge/github"
	"go.woodpecker-ci.org/woodpecker/v3/server/model"
	"golang.org/x/sync/errgroup"
)

type ProviderType string

const (
	ProviderTypeForge ProviderType = "forge"
	ProviderTypeFS    ProviderType = "fs"
)

// ForgeProvider wraps available woodpecker forges
type ForgeProvider struct {
	logger *slog.Logger
	forges map[model.ForgeType]forge.Forge
}

// NewForgeProvider returns a new ForgeProvider
func NewForgeProvider(logger *slog.Logger) (ForgeProvider, error) {
	forgeTypeGithub, err := github.New(github.Opts{
		URL:      "https://github.com",
		MergeRef: true,
	})
	if err != nil {
		return ForgeProvider{}, err
	}

	return ForgeProvider{
		logger: logger,
		forges: map[model.ForgeType]forge.Forge{
			model.ForgeTypeGithub: forgeTypeGithub,
		},
	}, nil
}

// Get returns the configuration file for the given environment
func (p ForgeProvider) Get(ctx context.Context, env Environment) ([]File, error) {
	f, ok := p.forges[env.Netrc.Type]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownType, env.Netrc.Type)
	}

	if env.Repo.Config == "" {
		return nil, ErrNoConfig
	}

	data, err := f.File(ctx, &model.User{
		AccessToken: env.Netrc.Login,
	}, env.Repo, env.Pipeline,
		// ce.Repo.Config must point to a configuration file, globs are not supported yet
		env.Repo.Config)
	if err != nil {
		return nil, err
	}

	return []File{{
		Name: env.Repo.Config,
		Data: data,
	}}, nil
}

// FSProvider provides configuration files from the filesystem
type FSProvider struct {
	logger *slog.Logger
	glob   string
	fs     fs.FS
}

// NewFSProvider returns a new FSProvider
func NewFSProvider(dir, glob string, logger *slog.Logger) (FSProvider, error) {
	dirFS := os.DirFS(dir)
	info, err := fs.Stat(dirFS, ".")
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return FSProvider{}, fmt.Errorf("CONFIG_SERVICE_PROVIDER_FS_SOURCE does not exist: %s", dir)
	case err != nil:
		return FSProvider{}, err
	}

	if !info.IsDir() {
		return FSProvider{}, fmt.Errorf("CONFIG_SERVICE_PROVIDER_FS_SOURCE is not a directory: %s", dir)
	}

	return FSProvider{
		logger: logger,
		glob:   glob,
		fs:     dirFS,
	}, err
}

// Get returns the configuration file for the given environment
func (p FSProvider) Get(_ context.Context, _ Environment) ([]File, error) {
	paths, err := fs.Glob(p.fs, p.glob)
	if err != nil {
		return nil, err
	}

	var files []File
	var eg errgroup.Group
	for _, fp := range paths {
		eg.Go(func() error {
			f, err := p.fs.Open(fp)
			if err != nil {
				return err
			}
			defer func() { _ = f.Close() }()

			buf := new(bytes.Buffer)
			if _, err := buf.ReadFrom(f); err != nil {
				return err
			}

			files = append(files, File{
				Name: fp,
				Data: buf.Bytes(),
			})

			return nil
		})
	}

	err = eg.Wait()
	return files, err
}
