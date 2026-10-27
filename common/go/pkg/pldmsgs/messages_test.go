// Copyright © 2026 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
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

package pldmsgs

import (
	"context"
	"testing"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/stretchr/testify/assert"
)

// TestMessagesRegistered exercises the package initializers (the pde/pdm closures
// and every message var declaration) by referencing a representative error and
// description message, then confirming each resolves through the i18n registry.
func TestMessagesRegistered(t *testing.T) {
	ctx := context.Background()

	// Error message: key matches the registered code and expands to its translation.
	assert.Equal(t, "PD020000", string(MsgContextCanceled))
	assert.Equal(t, "Context canceled", i18n.Expand(ctx, i18n.MessageKey(MsgContextCanceled)))

	// A status-hint-carrying error registers its hint.
	hint, ok := i18n.GetStatusHint(string(MsgTypesTimeParseFail))
	assert.True(t, ok)
	assert.Equal(t, 400, hint)

	// Description message resolves to its translation.
	assert.Equal(t, "IndexedBlock.number", string(IndexedBlockNumber))
	assert.Equal(t, "The block number", i18n.Expand(ctx, i18n.MessageKey(IndexedBlockNumber)))
}
