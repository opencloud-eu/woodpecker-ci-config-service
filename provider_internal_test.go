// Copyright 2026 OpenCloud GmbH
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
	"testing"

	"github.com/stretchr/testify/assert"
	"go.woodpecker-ci.org/woodpecker/v3/server/model"
)

func TestNetrcToken(t *testing.T) {
	t.Run("oauth app", func(t *testing.T) {
		assert.Equal(t, "oauth-token", netrcToken(model.Netrc{Login: "oauth-token", Password: "x-oauth-basic"}))
	})

	t.Run("github app", func(t *testing.T) {
		assert.Equal(t, "installation-token", netrcToken(model.Netrc{Login: "x-access-token", Password: "installation-token"}))
	})
}
