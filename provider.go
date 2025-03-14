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
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"sync"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/samber/lo"
	"go.woodpecker-ci.org/woodpecker/v3/server/forge"
	"go.woodpecker-ci.org/woodpecker/v3/server/forge/github"
	"go.woodpecker-ci.org/woodpecker/v3/server/model"
	"golang.org/x/sync/errgroup"
)

// Providers contains multiple converters.
type Providers []Provider

// Get returns the files by using the available providers.
func (providers Providers) Get(ctx context.Context, env Environment) ([]File, error) {
	var results []File
	for _, provider := range providers {
		data, err := provider.Get(ctx, env)
		switch {
		// not having a configuration is not a critical error per se...
		case errors.Is(err, ErrNoConfig):
			fallthrough
		// not knowing the type is not a critical error per se...
		case errors.Is(err, ErrUnknownType):
			// ... ignore the error and continue
			continue
		case err != nil:
			return nil, err
		}

		results = append(results, data...)
	}

	return results, nil
}

// ProviderType defines the type of the provider.
type ProviderType string

const (
	// ProviderTypeForge is the type for forge providers.
	ProviderTypeForge ProviderType = "forge"
	// ProviderTypeFS is the type for filesystem providers.
	ProviderTypeFS ProviderType = "fs"
)

// ForgeProvider wraps available woodpecker forges.
type ForgeProvider struct {
	logger *slog.Logger
	forges map[model.ForgeType]forge.Forge
}

// NewForgeProvider returns a new ForgeProvider.
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

// Get returns the configuration file for the given environment.
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
	}, &env.Repo, &env.Pipeline,
		// ce.Repo.Config must point to a configuration file, globs are not supported yet
		env.Repo.Config)
	if err != nil {
		return nil, err
	}

	return []File{{
		Name: env.Repo.Config,
		Data: string(data),
	}}, nil
}

// FSProvider provides configuration files from the filesystem.
type FSProvider struct {
	logger  *slog.Logger
	pattern string
	fs      fs.FS
}

// NewFSProvider returns a new FSProvider.
func NewFSProvider(p string, logger *slog.Logger) (FSProvider, error) {
	base, pattern := doublestar.SplitPattern(p)
	dirFS := os.DirFS(base)
	if _, err := fs.Stat(dirFS, "."); err != nil {
		return FSProvider{}, err
	}

	return FSProvider{
		logger:  logger,
		pattern: pattern,
		fs:      dirFS,
	}, nil
}

// Get returns the configuration file for the given environment.
func (p FSProvider) Get(_ context.Context, env Environment) ([]File, error) {
	paths, err := doublestar.Glob(p.fs, p.pattern)
	if err != nil {
		return nil, err
	}

	// consider all files if no configuration is set
	if env.Repo.Config == "" {
		env.Repo.Config = "**"
	}

	var files []File
	var mutex sync.Mutex
	var eg errgroup.Group
	for _, fp := range lo.Filter(paths, func(p string, _ int) bool {
		match, _ := doublestar.Match(env.Repo.Config, p)
		return match
	}) {
		eg.Go(func() error {
			f, err := p.fs.Open(fp)
			if err != nil {
				return err
			}
			defer func() {
				_ = f.Close()
			}()

			buf := new(bytes.Buffer)
			if _, err := buf.ReadFrom(f); err != nil {
				return err
			}

			mutex.Lock()
			files = append(files, File{
				Name: fp,
				Data: buf.String(),
			})
			mutex.Unlock()

			return nil
		})
	}

	err = eg.Wait()
	return files, err
}
