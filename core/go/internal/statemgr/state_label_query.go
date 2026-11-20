// Copyright contributors to Paladin, an LFDT project
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

package statemgr

import (
	"slices"

	"github.com/LFDT-Paladin/paladin/core/internal/filters"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
)

// labelIndexDriver picks the label an available-state query is driven from: the first positive,
// case-sensitive equality on an indexed label among the top-level predicates (equal before eq, each
// in the order listed). Its row's partial index seeks straight to that label value's available
// states in created order, and every other predicate is checked per row, so a domain gets the
// narrowest scan by listing its most selective label first. Equalities inside OR branches cannot
// drive, as they do not hold for every row.
func labelIndexDriver(jq *query.QueryJSON, labels map[string]*schemaLabelInfo) *schemaLabelInfo {
	for _, op := range slices.Concat(jq.Equal, jq.Eq) {
		if op.Not || op.CaseInsensitive {
			continue
		}
		if fi := labels[op.Field]; fi != nil {
			return fi
		}
	}
	return nil
}

func labelTable(fi *schemaLabelInfo) string {
	if fi.labelType == labelTypeInt64 || fi.labelType == labelTypeBool {
		return "state_int64_labels"
	}
	return "state_labels"
}

// labelRowFieldSet resolves the state's own fields to the driving label row's copies of them, so
// bounds and sorts on them are served by that row's index. It must cover every field in
// baseStateFields.
type labelRowFieldSet struct {
	*trackingLabelSet
	row string
}

func (fs labelRowFieldSet) ResolverFor(fieldName string) filters.FieldResolver {
	switch fieldName {
	case ".id":
		return filters.HexBytesField(fs.row + `."state"`)
	case ".created":
		return filters.TimestampField(fs.row + `."created"`)
	case "contractAddress":
		return filters.HexBytesField(fs.row + `."contract_address"`)
	}
	return fs.trackingLabelSet.ResolverFor(fieldName)
}
