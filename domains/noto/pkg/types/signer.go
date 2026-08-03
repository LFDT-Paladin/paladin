/*
 * Copyright © 2026 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package types

import "fmt"

// NullifierSpec payload type: NotoCoin state JSON signing logic
// is used to derive a spend nullifier for the target state
const PAYLOAD_DOMAIN_NOTO_NULLIFIER = "domain:noto:nullifier"

// NullifierSpec verifier type: placeholder (nullifier derivation requires no external key)
const VERIFIER_DOMAIN_NOTO_NULLIFIER = "domain:noto:nullifier:verifier"

func AlgoDomainNullifier(name string) string {
	return fmt.Sprintf("domain:%s:nullifier", name)
}
