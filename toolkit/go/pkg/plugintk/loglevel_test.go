// Copyright contributors to Paladin, an LFDT project
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package plugintk

import (
	"testing"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/stretchr/testify/assert"
)

func TestApplyLogLevel(t *testing.T) {
	log.SetLevel("info")

	// empty leaves the current level untouched (older engine sends no field)
	applyLogLevel("")
	assert.Equal(t, "info", log.GetLevel())

	// a set level drives log.SetLevel
	applyLogLevel("error")
	assert.Equal(t, "error", log.GetLevel())

	log.SetLevel("info")
}

func TestEntrypointDefaultLevelIsInfo(t *testing.T) {
	log.SetLevel("error")
	NewPluginLibraryEntrypoint(func() PluginBase { return nil })
	assert.Equal(t, "info", log.GetLevel())
	log.SetLevel("info")
}
