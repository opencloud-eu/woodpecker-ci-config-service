package wcs_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/stretchr/testify/assert"

	wcs "github.com/opencloud-eu/woodpecker-ci-config-service"
)

func TestNewFSProvider(t *testing.T) {
	tempdir := t.TempDir()
	tempfile, err := os.CreateTemp(tempdir, "test.star")
	assert.NoError(t, err)
	defer func() { _ = tempfile.Close() }()

	t.Run("fails if the source does not exist", func(t *testing.T) {
		_, err := wcs.NewFSProvider(filepath.Join(tempdir, "unknown", "unknown.star"), noopLogger)
		assert.ErrorIs(t, err, fs.ErrNotExist)
	})

	t.Run("fails if the source is not a directory", func(t *testing.T) {
		_, err := wcs.NewFSProvider(filepath.Join(tempfile.Name(), "unknown.star"), noopLogger)
		assert.ErrorIs(t, err, syscall.ENOTDIR)
	})

	t.Run("pass", func(t *testing.T) {
		_, err := wcs.NewFSProvider(tempfile.Name(), noopLogger)
		assert.NoError(t, err)
	})
}

func TestFSProvider_Get(t *testing.T) {
	ctx := context.Background()
	env := wcs.Environment{}
	tempdir := t.TempDir()
	tempfileStar, err := os.CreateTemp(tempdir, "test.star")
	assert.NoError(t, err)
	defer func() { _ = tempfileStar.Close() }()
	wcs.Must1(tempfileStar.Write([]byte(tempfileStar.Name())))

	tempfileYaml, err := os.CreateTemp(tempdir, "test.yaml")
	assert.NoError(t, err)
	defer func() { _ = tempfileYaml.Close() }()
	wcs.Must1(tempfileYaml.Write([]byte(tempfileYaml.Name())))

	tempfileIgnored, err := os.CreateTemp(tempdir, "test.xlsx")
	assert.NoError(t, err)
	defer func() { _ = tempfileIgnored.Close() }()

	t.Run("fails on invalid glob pattern", func(t *testing.T) {
		provider, err := wcs.NewFSProvider(filepath.Join(tempdir, "/*/[]a]"), noopLogger)
		assert.NoError(t, err)

		_, err = provider.Get(ctx, env)
		assert.ErrorIs(t, err, doublestar.ErrBadPattern)
	})

	t.Run("passes", func(t *testing.T) {
		provider, err := wcs.NewFSProvider(filepath.Join(tempdir, "/*.{yaml,star}*"), noopLogger)
		assert.NoError(t, err)

		files, err := provider.Get(ctx, env)
		assert.NoError(t, err)

		matches := map[string]wcs.File{}
		for _, file := range files {
			matches[file.Name] = file
		}

		assert.Len(t, matches, 2)
		assert.Equal(t, matches[wcs.Must1(filepath.Rel(tempdir, tempfileStar.Name()))].Data, tempfileStar.Name())
		assert.Equal(t, matches[wcs.Must1(filepath.Rel(tempdir, tempfileYaml.Name()))].Data, tempfileYaml.Name())
	})
}
