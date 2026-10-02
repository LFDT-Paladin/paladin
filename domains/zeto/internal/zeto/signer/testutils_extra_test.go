/*
 * Copyright © 2026 Kaleido, Inc.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package signer

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTestWitnessCalculator_Methods(t *testing.T) {
	calc := &testWitnessCalculator{}
	_, err := calc.CalculateWitness(map[string]interface{}{"x": 1}, true)
	require.NoError(t, err)
	_, err = calc.CalculateBinWitness(map[string]interface{}{"x": 1}, true)
	require.NoError(t, err)
	_, err = calc.CalculateWTNSBin(map[string]interface{}{"x": 1}, false)
	require.NoError(t, err)
}
