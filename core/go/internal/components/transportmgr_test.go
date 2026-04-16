/*
 * Copyright © 2024 Kaleido, Inc.
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

package components

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransportSendOptions_ErrorHandler_CanBeSetAndCalled(t *testing.T) {
	ctx := context.Background()
	testErr := errors.New("test error")

	var capturedCtx context.Context
	var capturedErr error
	handlerCalled := false

	errorHandler := func(ctx context.Context, err error) {
		capturedCtx = ctx
		capturedErr = err
		handlerCalled = true
	}

	options := &TransportSendOptions{
		ErrorHandler: errorHandler,
	}

	require.NotNil(t, options.ErrorHandler)
	options.ErrorHandler(ctx, testErr)

	assert.True(t, handlerCalled)
	assert.Equal(t, ctx, capturedCtx)
	assert.Equal(t, testErr, capturedErr)
}

func TestTransportSendOptions_ErrorHandler_CanBeNil(t *testing.T) {
	options := &TransportSendOptions{
		ErrorHandler: nil,
	}

	assert.Nil(t, options.ErrorHandler)
}

func TestTransportSendOptions_ErrorHandler_ReceivesCorrectContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), "test-key", "test-value")
	testErr := errors.New("test error")

	var capturedCtx context.Context

	errorHandler := func(ctx context.Context, err error) {
		capturedCtx = ctx
	}

	options := &TransportSendOptions{
		ErrorHandler: errorHandler,
	}

	options.ErrorHandler(ctx, testErr)

	assert.Equal(t, "test-value", capturedCtx.Value("test-key"))
}

func TestTransportSendOptions_ErrorHandler_ReceivesCorrectError(t *testing.T) {
	ctx := context.Background()
	testErr1 := errors.New("error 1")
	testErr2 := errors.New("error 2")

	var capturedErr error

	errorHandler := func(ctx context.Context, err error) {
		capturedErr = err
	}

	options := &TransportSendOptions{
		ErrorHandler: errorHandler,
	}

	options.ErrorHandler(ctx, testErr1)
	assert.Equal(t, testErr1, capturedErr)

	options.ErrorHandler(ctx, testErr2)
	assert.Equal(t, testErr2, capturedErr)
}

func TestTransportSendOptions_ErrorHandler_CanBeCalledMultipleTimes(t *testing.T) {
	ctx := context.Background()
	callCount := 0

	errorHandler := func(ctx context.Context, err error) {
		callCount++
	}

	options := &TransportSendOptions{
		ErrorHandler: errorHandler,
	}

	options.ErrorHandler(ctx, errors.New("error 1"))
	options.ErrorHandler(ctx, errors.New("error 2"))
	options.ErrorHandler(ctx, errors.New("error 3"))

	assert.Equal(t, 3, callCount)
}

func TestTransportSendOptions_ErrorHandler_WithNilError(t *testing.T) {
	ctx := context.Background()
	var capturedErr error

	errorHandler := func(ctx context.Context, err error) {
		capturedErr = err
	}

	options := &TransportSendOptions{
		ErrorHandler: errorHandler,
	}

	options.ErrorHandler(ctx, nil)

	assert.Nil(t, capturedErr)
}

func TestTransportSendOptions_CanBeCreatedWithoutErrorHandler(t *testing.T) {
	options := &TransportSendOptions{}

	assert.Nil(t, options.ErrorHandler)
}

