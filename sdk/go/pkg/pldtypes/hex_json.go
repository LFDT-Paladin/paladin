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

package pldtypes

import (
	"encoding/json"
	"slices"
)

// jsonNumericText classifies a JSON scalar that is already fully in b and
// returns the inner text to hand to a numeric string parser.
//   - quoted string  -> the unquoted contents  (ok=true)
//   - JSON number    -> the literal digits      (ok=true)
//   - anything else (object/array/true/false/null) -> ok=false
//
// The outer json scanner has already validated b is a single well-formed
// value, so we only need to distinguish the two accepted shapes. This avoids
// the per-call json.Decoder/bytes.Reader/interface{}/json.Number allocations
// that a Decode into interface{} incurs.
func jsonNumericText(b []byte) (text string, ok bool) {
	// Trim leading/trailing JSON whitespace (space, tab, CR, LF).
	i, j := 0, len(b)
	for i < j && isJSONSpace(b[i]) {
		i++
	}
	for j > i && isJSONSpace(b[j-1]) {
		j--
	}
	if i >= j {
		return "", false
	}
	s := b[i:j]

	if s[0] == '"' {
		// Fast path: no backslash means the contents are a verbatim slice.
		inner := s[1 : len(s)-1] // scanner guarantees a closing quote
		if !hasByte(inner, '\\') {
			return string(inner), true
		}
		// Cold path: unescape via the stdlib into a string.
		var str string
		if err := json.Unmarshal(s, &str); err != nil {
			return "", false
		}
		return str, true
	}

	// JSON number: first byte is '-' or a digit.
	if s[0] == '-' || (s[0] >= '0' && s[0] <= '9') {
		return string(s), true
	}

	return "", false // object/array/true/false/null
}

func isJSONSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

func hasByte(b []byte, c byte) bool {
	return slices.Contains(b, c)
}
