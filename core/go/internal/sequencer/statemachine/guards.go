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

// Not returns a guard that negates the result of the given guard
func Not[T any](guard Guard[T]) Guard[T] {
	return func(ctx context.Context, subject T) bool {
		return !guard(ctx, subject)
	}
}

// And returns a guard that returns true only if all given guards return true
// Short-circuits on first false result
func And[T any](guards ...Guard[T]) Guard[T] {
	return func(ctx context.Context, subject T) bool {
		for _, guard := range guards {
			if !guard(ctx, subject) {
				return false
			}
		}
		return true
	}
}

// Or returns a guard that returns true if any of the given guards returns true
// Short-circuits on first true result
func Or[T any](guards ...Guard[T]) Guard[T] {
	return func(ctx context.Context, subject T) bool {
		for _, guard := range guards {
			if guard(ctx, subject) {
				return true
			}
		}
		return false
	}
}
