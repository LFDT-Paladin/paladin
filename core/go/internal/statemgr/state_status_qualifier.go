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

package statemgr

import (
	"fmt"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"gorm.io/gorm"
)

// whereClauseForQual scopes a query to the states matching a status qualifier, expressed through the
// Confirmed join and the given spend join. Confirmed is a synonym of available - a state is
// confirmed for use only while it is also unspent - so both select the same states.
func whereClauseForQual(db *gorm.DB /* must be the DB not the query */, q pldapi.StateStatusQualifier, spentColumn string) *gorm.DB {
	switch q {
	case pldapi.StateStatusAvailable, pldapi.StateStatusConfirmed:
		return db.
			Where(fmt.Sprintf(`"%s"."transaction" IS NULL`, spentColumn)).
			Where(`"Confirmed"."transaction" IS NOT NULL`)
	case pldapi.StateStatusUnconfirmed:
		return db.Where(`"Confirmed"."transaction" IS NULL`)
	case pldapi.StateStatusSpent:
		return db.Where(fmt.Sprintf(`"%s"."transaction" IS NOT NULL`, spentColumn))
	default: // pldapi.StateStatusAll, which is also what the reader defaults an unset qualifier to
		return db.Where("TRUE")
	}
}
