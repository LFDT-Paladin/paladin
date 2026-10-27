// Copyright © 2024 Kaleido, Inc.
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

package pldtypes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJSONNumericText(t *testing.T) {
	// Leading/trailing whitespace is trimmed around a number.
	text, ok := jsonNumericText([]byte(" \t\n\r 123 \t\n\r "))
	assert.True(t, ok)
	assert.Equal(t, "123", text)

	// Leading/trailing whitespace is trimmed around a quoted string.
	text, ok = jsonNumericText([]byte(`  "0x10"  `))
	assert.True(t, ok)
	assert.Equal(t, "0x10", text)

	// Empty / whitespace-only input.
	_, ok = jsonNumericText([]byte("   "))
	assert.False(t, ok)
	_, ok = jsonNumericText([]byte(""))
	assert.False(t, ok)

	// Negative number.
	text, ok = jsonNumericText([]byte("-42"))
	assert.True(t, ok)
	assert.Equal(t, "-42", text)

	// Cold path: quoted string containing an escape is unescaped via stdlib.
	// "10" decodes to "10".
	text, ok = jsonNumericText([]byte("\"1\\u0030\""))
	assert.True(t, ok)
	assert.Equal(t, "10", text)

	// Cold path: invalid escape sequence fails to unmarshal.
	_, ok = jsonNumericText([]byte("\"\\x\""))
	assert.False(t, ok)

	// Non-numeric scalars are rejected.
	_, ok = jsonNumericText([]byte("true"))
	assert.False(t, ok)
	_, ok = jsonNumericText([]byte("null"))
	assert.False(t, ok)
	_, ok = jsonNumericText([]byte("{}"))
	assert.False(t, ok)
}
