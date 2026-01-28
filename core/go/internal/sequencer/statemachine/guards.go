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

package statemachine

import "context"

// Not returns a guard that negates the given guard.
// If the input guard returns true, Not returns false, and vice versa.
func Not[E any](guard Guard[E]) Guard[E] {
	return func(ctx context.Context, entity E) bool {
		return !guard(ctx, entity)
	}
}

// And returns a guard that combines multiple guards with logical AND.
// The resulting guard returns true only if all input guards return true.
// Guards are evaluated in order; evaluation stops at the first false result.
func And[E any](guards ...Guard[E]) Guard[E] {
	return func(ctx context.Context, entity E) bool {
		for _, guard := range guards {
			if !guard(ctx, entity) {
				return false
			}
		}
		return true
	}
}

// Or returns a guard that combines multiple guards with logical OR.
// The resulting guard returns true if any of the input guards return true.
// Guards are evaluated in order; evaluation stops at the first true result.
func Or[E any](guards ...Guard[E]) Guard[E] {
	return func(ctx context.Context, entity E) bool {
		for _, guard := range guards {
			if guard(ctx, entity) {
				return true
			}
		}
		return false
	}
}
