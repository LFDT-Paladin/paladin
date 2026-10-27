// Copyright © 2026 Kaleido, Inc.
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

//go:build !reference
// +build !reference

package reference

import (
	"context"
	"testing"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/stretchr/testify/assert"
	"golang.org/x/text/language"
)

func TestGenerateAllReferenceMarkdown(t *testing.T) {
	ctx := i18n.WithLang(context.Background(), language.AmericanEnglish)
	allDocs, err := GenerateAllReferenceMarkdown(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, allDocs)
	assert.NotEmpty(t, allDocs)
}

func TestGenerateAllReferenceMarkdownMapsCorrectly(t *testing.T) {
	ctx := i18n.WithLang(context.Background(), language.AmericanEnglish)
	apiDocs, err := GenerateAPIObjectsReferenceMarkdown(ctx)
	assert.NoError(t, err)

	configDocs, err := GenerateConfigReferenceMarkdown(ctx)
	assert.NoError(t, err)

	allDocs, err := GenerateAllReferenceMarkdown(ctx)
	assert.NoError(t, err)

	// Verify all API docs are in the combined result
	for key := range apiDocs {
		assert.Contains(t, allDocs, key, "API doc %s should be in combined docs", key)
	}

	// Verify all config docs are in the combined result
	for key := range configDocs {
		assert.Contains(t, allDocs, key, "Config doc %s should be in combined docs", key)
	}
}
