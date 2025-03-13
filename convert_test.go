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

package wccs_test

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.woodpecker-ci.org/woodpecker/v3/server/model"
	"gopkg.in/yaml.v3"

	wccs "github.com/opencloud-eu/woodpecker-ci-config-service"
)

//go:embed testdata/environment.star
var environmentStar string

func TestStarlarkConverter_Compatible(t *testing.T) {
	c, err := wccs.NewStarlarkConverter(noopLogger)
	assert.Nil(t, err)
	assert.Equal(t, true, c.Compatible(wccs.File{Name: "test.star"}))
	assert.Equal(t, false, c.Compatible(wccs.File{Name: "test.start"}))
	assert.Equal(t, false, c.Compatible(wccs.File{Name: "test.sta"}))
	assert.Equal(t, false, c.Compatible(wccs.File{Name: "test.yaml"}))
}

func TestStarlarkConverter_Convert(t *testing.T) {
	c, err := wccs.NewStarlarkConverter(noopLogger)
	assert.Nil(t, err)

	t.Run("fails without content", func(t *testing.T) {
		_, err := c.Convert(wccs.File{}, wccs.Environment{})
		assert.ErrorIs(t, err, wccs.ErrNoContent)
	})

	t.Run("fails if the main entrypoint does not exist", func(t *testing.T) {
		_, err := c.Convert(wccs.File{Data: `foo = "bar"`}, wccs.Environment{})
		assert.ErrorIs(t, err, wccs.ErrNoEntrypoint)
	})

	t.Run("fails without a name", func(t *testing.T) {
		_, err := c.Convert(wccs.File{Data: environmentStar}, wccs.Environment{})
		assert.ErrorIs(t, err, wccs.ErrMissingParam)
		assert.Contains(t, err.Error(), "name")
	})

	t.Run("adds the YAML extension", func(t *testing.T) {
		build := func(name string) wccs.File {
			files, err := c.Convert(wccs.File{Data: environmentStar}, wccs.Environment{Repo: model.Repo{Name: name}})
			assert.Nil(t, err)
			assert.Len(t, files, 1)
			return files[0]
		}

		assert.Equal(t, "testing.yaml", build("testing").Name)
		assert.Equal(t, "testing.yaml", build("testing.json").Name)
	})

	t.Run("converts the environment", func(t *testing.T) {
		files, err := c.Convert(wccs.File{Data: environmentStar}, wccs.Environment{Repo: model.Repo{Name: "testing"}, Pipeline: model.Pipeline{Title: "tests"}})
		assert.Nil(t, err)
		assert.Len(t, files, 1)
		file := files[0]
		data := struct {
			Name  string
			Title string
			False bool
			True  bool
			None  []any
		}{}
		assert.Nil(t, yaml.Unmarshal([]byte(file.Data), &data))

		t.Run("deletes the name field", func(t *testing.T) {
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
