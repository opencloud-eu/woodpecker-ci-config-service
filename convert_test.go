package wcs_test

import (
	_ "embed"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.woodpecker-ci.org/woodpecker/v3/server/model"
	"gopkg.in/yaml.v3"

	wcs "github.com/opencloud-eu/woodpecker-ci-config-service"
)

var (
	noopLogger = slog.New(slog.DiscardHandler)
	//go:embed testdata/environment.star
	environmentStar []byte
)

func TestStarlarkConverter_Compatible(t *testing.T) {
	c, err := wcs.NewStarlarkConverter(noopLogger)
	assert.Nil(t, err)
	assert.Equal(t, true, c.Compatible(wcs.File{Name: "test.star"}))
	assert.Equal(t, false, c.Compatible(wcs.File{Name: "test.start"}))
	assert.Equal(t, false, c.Compatible(wcs.File{Name: "test.sta"}))
	assert.Equal(t, false, c.Compatible(wcs.File{Name: "test.yaml"}))
}

func TestStarlarkConverter_Convert(t *testing.T) {
	c, err := wcs.NewStarlarkConverter(noopLogger)
	assert.Nil(t, err)

	t.Run("fails without content", func(t *testing.T) {
		_, err := c.Convert(wcs.File{}, wcs.Environment{})
		assert.ErrorIs(t, err, wcs.ErrNoContent)
	})

	t.Run("fails if the main entrypoint does not exist", func(t *testing.T) {
		_, err := c.Convert(wcs.File{Data: []byte(`foo = "bar"`)}, wcs.Environment{})
		assert.ErrorIs(t, err, wcs.ErrNoEntrypoint)
	})

	t.Run("fails without a name", func(t *testing.T) {
		_, err := c.Convert(wcs.File{Data: environmentStar}, wcs.Environment{})
		assert.ErrorIs(t, err, wcs.ErrMissingParam)
		assert.Contains(t, err.Error(), "name")
	})

	t.Run("converts the environment", func(t *testing.T) {
		files, err := c.Convert(wcs.File{Data: environmentStar}, wcs.Environment{Repo: model.Repo{Name: "testing"}, Pipeline: model.Pipeline{Title: "tests"}})
		assert.Nil(t, err)
		assert.Len(t, files, 1)
		file := files[0]
		data := struct {
			Name  string
			Title string
			False bool
			True  bool
			None  []interface{}
		}{}
		assert.Nil(t, yaml.Unmarshal(file.Data, &data))

		t.Run("deletes the name field", func(t *testing.T) {
			assert.Equal(t, "testing", file.Name)
			assert.Empty(t, data.Name)
		})

		t.Run("keeps other fields", func(t *testing.T) {
			assert.Equal(t, "tests", data.Title)
		})

		t.Run("replaces keywords", func(t *testing.T) {
			assert.False(t, data.False)
			assert.True(t, data.True)
			assert.Len(t, data.None, 0)
		})
	})
}
