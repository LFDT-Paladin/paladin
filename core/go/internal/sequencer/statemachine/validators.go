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

import (
	"context"

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
)

// ValidatorAnd combines validators with logical AND.
// Validators are evaluated in order and short-circuit on first false or error.
func ValidatorAnd[E any](validators ...Validator[E]) Validator[E] {
	return func(ctx context.Context, entity E, event common.Event) (bool, error) {
		for _, validator := range validators {
			valid, err := validator(ctx, entity, event)
			if err != nil {
				return false, err
			}
			if !valid {
				return false, nil
			}
		}
		return true, nil
	}
}

// ValidatorOr combines validators with logical OR.
// Validators are evaluated in order and short-circuit on first true or error.
func ValidatorOr[E any](validators ...Validator[E]) Validator[E] {
	return func(ctx context.Context, entity E, event common.Event) (bool, error) {
		for _, validator := range validators {
			valid, err := validator(ctx, entity, event)
			if err != nil {
				return false, err
			}
			if valid {
				return true, nil
			}
		}
		return false, nil
	}
}

// ValidatorNot negates the result of a validator.
// Errors from the inner validator are propagated unchanged.
func ValidatorNot[E any](validator Validator[E]) Validator[E] {
	return func(ctx context.Context, entity E, event common.Event) (bool, error) {
		valid, err := validator(ctx, entity, event)
		if err != nil {
			return false, err
		}
		return !valid, nil
	}
}
