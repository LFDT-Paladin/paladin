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
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
)

// applyLogLevel sets the plugin runtime's log level from the level supplied by the
// engine on a Configure*Request. An empty value leaves the entrypoint default in place,
// keeping an older engine (that never sends the field) backward compatible.
func applyLogLevel(logLevel string) {
	if logLevel != "" {
		log.SetLevel(logLevel)
	}
}
