/*
 * Copyright © 2025 Kaleido, Inc.
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

package keymanagersigningmodule

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/LFDT-Paladin/paladin/toolkit/pkg/algorithms"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/signpayloads"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/verifiers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test utilities and common setup

type testCallbacks struct{}

type handler func(r *http.Request) (int, interface{})

// Common test constants
const (
	testKeyID         = "k:qh6ujpp81h"
	testKeyName       = "key1"
	testWalletName    = "parent"
	testEthAddress    = "0x24facb8bf8117426427115c73d4f51eb631a6152"
	testPayload       = "some data"
	testPayloadBase64 = "c29tZSBkYXRh" // base64 of "some data"
)

// Common test data
var (
	testPathSegments = []*prototk.ResolveKeyPathSegment{
		{
			Name:  testWalletName,
			Index: 0,
		},
	}

	testRequiredIdentifiers = []*prototk.PublicKeyIdentifierType{
		{
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		},
	}

	testSignRequest = &prototk.SignWithKeyRequest{
		KeyHandle:   testKeyID,
		Algorithm:   algorithms.ECDSA_SECP256K1,
		PayloadType: signpayloads.OPAQUE_TO_RSV,
		Payload:     []byte(testPayload),
	}
)

// Utility functions

func newTestServer(t *testing.T, handler handler) (context.Context, string, func()) {
	ctx, cancelCtx := context.WithCancel(context.Background())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status, res := handler(r)

		b := []byte(`{}`)
		var err error
		if res != nil {
			b, err = json.Marshal(res)
			assert.NoError(t, err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", strconv.Itoa(len(b)))
		w.WriteHeader(status)
		_, _ = w.Write(b)
	}))

	serverURL := fmt.Sprintf("http://%s", server.Listener.Addr())
	return ctx, serverURL, func() {
		cancelCtx()
		server.Close()
	}
}

func newTestSigningModule(t *testing.T) *keyManagerSigningModule {
	callbacks := &testCallbacks{}
	return NewKeyManagerSigningModule(callbacks).(*keyManagerSigningModule)
}

func configureTestSigningModule(t *testing.T, ctx context.Context, signingModule *keyManagerSigningModule, httpEndpoint string, configExtras ...string) {
	config := fmt.Sprintf(`{"httpConfig":{"url":"%s"}}`, httpEndpoint)

	// Add any extra configuration
	for _, extra := range configExtras {
		config = config[:len(config)-1] + "," + extra + "}"
	}

	_, err := signingModule.ConfigureSigningModule(ctx, &prototk.ConfigureSigningModuleRequest{
		Name:       "test",
		ConfigJson: config,
	})
	require.NoError(t, err)
}

func configureTestSigningModuleWithBgCtx(t *testing.T, signingModule *keyManagerSigningModule, httpEndpoint string, configExtras ...string) {
	configureTestSigningModule(t, signingModule.bgCtx, signingModule, httpEndpoint, configExtras...)
}

func createResolveKeyResponse(keyID string, ethAddress string) *ResolveKeyResponse {
	return &ResolveKeyResponse{
		ID: keyID,
		Identifiers: []*PublicIdentifier{
			{
				Type:  ADDRESS_ETH,
				Value: ethAddress,
			},
		},
	}
}

func createErrorResponse(errorMsg string) *ErrorResponse {
	return &ErrorResponse{
		Error: errorMsg,
	}
}

func createSignResponse(payload string) *SignResponse {
	return &SignResponse{
		Payload: payload,
	}
}

func validateResolveKeyRequest(t *testing.T, r *http.Request) ResolveKeyRequest {
	assert.Equal(t, "POST", r.Method)
	assert.Equal(t, "/resolve", r.URL.Path)

	var resolveReq ResolveKeyRequest
	err := json.NewDecoder(r.Body).Decode(&resolveReq)
	assert.NoError(t, err)

	require.Len(t, resolveReq.PublicIdentifierTypesToResolve, 1)
	assert.Equal(t, ADDRESS_ETH, resolveReq.PublicIdentifierTypesToResolve[0])
	assert.Equal(t, "kld:///keystore/parent/key/key1", *resolveReq.KeyIdentifier.KeyURI)

	return resolveReq
}

func validateSignRequest(t *testing.T, r *http.Request) SignRequest {
	assert.Equal(t, "POST", r.Method)
	assert.Equal(t, "/sign", r.URL.Path)

	var signReq SignRequest
	err := json.NewDecoder(r.Body).Decode(&signReq)
	assert.NoError(t, err)

	// Validate the request
	assert.NotNil(t, signReq.KeyIdentifier)
	assert.NotNil(t, signReq.KeyIdentifier.KeyID)
	assert.Equal(t, testPayloadBase64, signReq.Payload)
	assert.Equal(t, ECDSA_256, signReq.SigningAlgorithm)
	assert.Equal(t, DIGEST_TO_RSV, signReq.SigningFormat)

	return signReq
}

// Test cases

func TestPluginLifecycle(t *testing.T) {
	pb := NewPlugin(context.Background())
	assert.NotNil(t, pb)
}

func TestBadConfigJSON(t *testing.T) {
	signingModule := newTestSigningModule(t)
	_, err := signingModule.ConfigureSigningModule(signingModule.bgCtx, &prototk.ConfigureSigningModuleRequest{
		Name:       "test",
		ConfigJson: `{!!!!`,
	})
	assert.Regexp(t, "KA180001", err)
}

func TestInvalidConfigJSON(t *testing.T) {
	signingModule := newTestSigningModule(t)
	_, err := signingModule.ConfigureSigningModule(signingModule.bgCtx, &prototk.ConfigureSigningModuleRequest{
		Name:       "test",
		ConfigJson: `{"httpConfig":{"url": "notvalidurl"}}`,
	})
	assert.Regexp(t, "PD020501", err)
}

func TestGoodConfigJSON(t *testing.T) {
	signingModule := newTestSigningModule(t)
	res, err := signingModule.ConfigureSigningModule(signingModule.bgCtx, &prototk.ConfigureSigningModuleRequest{
		Name:       "grpc",
		ConfigJson: `{"httpConfig":{"url": "https://jsonrpcendpoint"}}`,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
}

func TestGoodConfigJSONWithFolderPath(t *testing.T) {
	signingModule := newTestSigningModule(t)
	res, err := signingModule.ConfigureSigningModule(signingModule.bgCtx, &prototk.ConfigureSigningModuleRequest{
		Name: "grpc",
		ConfigJson: `{
			"httpConfig": {"url": "https://jsonrpcendpoint"},
			"keystoreName": "mykeystore",
			"folderPath": "base/path"
		}`,
	})
	require.NoError(t, err)
	require.NotNil(t, res)

	// Verify the configuration was parsed correctly
	assert.Equal(t, "mykeystore", signingModule.keystoreName)
	assert.Equal(t, "base/path", signingModule.folderPath)
}

func TestConfigJSONWithFolderPathWithoutKeystoreName(t *testing.T) {
	signingModule := newTestSigningModule(t)
	_, err := signingModule.ConfigureSigningModule(signingModule.bgCtx, &prototk.ConfigureSigningModuleRequest{
		Name: "grpc",
		ConfigJson: `{
			"httpConfig": {"url": "https://jsonrpcendpoint"},
			"folderPath": "base/path"
		}`,
	})
	require.Error(t, err)
	require.Regexp(t, "KA180014", err) // MsgFolderPathRequiresKeystoreName
}

func TestConfigJSONWithFolderPathWithEmptyKeystoreName(t *testing.T) {
	signingModule := newTestSigningModule(t)
	_, err := signingModule.ConfigureSigningModule(signingModule.bgCtx, &prototk.ConfigureSigningModuleRequest{
		Name: "grpc",
		ConfigJson: `{
			"httpConfig": {"url": "https://jsonrpcendpoint"},
			"keystoreName": "",
			"folderPath": "base/path"
		}`,
	})
	require.Error(t, err)
	require.Regexp(t, "KA180014", err) // MsgFolderPathRequiresKeystoreName
}

func TestResolveKeyOk(t *testing.T) {
	ctx, httpEndpoint, done := newTestServer(t, func(r *http.Request) (int, interface{}) {
		resolveReq := validateResolveKeyRequest(t, r)

		// Verify autoKeyCreation and KeySpec are always set
		assert.True(t, resolveReq.AutoKeyCreation)
		assert.Equal(t, EC_NIST_P256, resolveReq.KeySpec)

		return 200, createResolveKeyResponse(testKeyID, testEthAddress)
	})
	defer done()

	signingModule := newTestSigningModule(t)
	configureTestSigningModule(t, ctx, signingModule, httpEndpoint)

	res, err := signingModule.ResolveKey(ctx, &prototk.ResolveKeyRequest{
		Name:                testKeyName,
		Path:                testPathSegments,
		RequiredIdentifiers: testRequiredIdentifiers,
	})
	require.NoError(t, err)
	assert.Equal(t, testKeyID, res.KeyHandle)
}

func TestResolveKeyEmptyNameError(t *testing.T) {
	ctx := context.Background()
	signingModule := newTestSigningModule(t)
	configureTestSigningModule(t, ctx, signingModule, "http://testendpoint")

	_, err := signingModule.ResolveKey(signingModule.bgCtx, &prototk.ResolveKeyRequest{})
	require.Regexp(t, "KA180002", err)
}

func TestResolveKeyNoIdentifierError(t *testing.T) {
	ctx := context.Background()
	signingModule := newTestSigningModule(t)
	configureTestSigningModule(t, ctx, signingModule, "http://testendpoint")

	_, err := signingModule.ResolveKey(ctx, &prototk.ResolveKeyRequest{
		Name: testKeyName,
		Path: testPathSegments,
	})
	require.Regexp(t, "KA180003", err)
}

func TestResolveKeyInvalidIdentifierError(t *testing.T) {
	ctx := context.Background()
	signingModule := newTestSigningModule(t)
	configureTestSigningModule(t, ctx, signingModule, "http://testendpoint")

	_, err := signingModule.ResolveKey(ctx, &prototk.ResolveKeyRequest{
		Name: testKeyName,
		Path: testPathSegments,
		RequiredIdentifiers: []*prototk.PublicKeyIdentifierType{
			{
				Algorithm:    algorithms.ECDSA_SECP256K1,
				VerifierType: "unsupported",
			},
		},
	})
	require.Regexp(t, "KA180004", err)
}

func TestResolveKeyNoPathsError(t *testing.T) {
	ctx := context.Background()
	signingModule := newTestSigningModule(t)
	configureTestSigningModule(t, ctx, signingModule, "http://testendpoint")

	_, err := signingModule.ResolveKey(ctx, &prototk.ResolveKeyRequest{
		Name:                testKeyName,
		RequiredIdentifiers: testRequiredIdentifiers,
	})
	require.Regexp(t, "KA180005", err)
}

func TestResolveKeyResolveError(t *testing.T) {
	ctx, httpEndpoint, done := newTestServer(t, func(r *http.Request) (int, interface{}) {
		validateResolveKeyRequest(t, r)
		return 500, createErrorResponse("resolve error")
	})
	defer done()

	signingModule := newTestSigningModule(t)
	configureTestSigningModule(t, ctx, signingModule, httpEndpoint)

	_, err := signingModule.ResolveKey(ctx, &prototk.ResolveKeyRequest{
		Name:                testKeyName,
		Path:                testPathSegments,
		RequiredIdentifiers: testRequiredIdentifiers,
	})
	require.Regexp(t, "KA180010", err)
	require.Regexp(t, "resolve error", err)
}

func TestResolveKeyNoIdentifiersError(t *testing.T) {
	ctx, httpEndpoint, done := newTestServer(t, func(r *http.Request) (int, interface{}) {
		validateResolveKeyRequest(t, r)

		// Return a successful response but with no identifiers
		response := &ResolveKeyResponse{
			ID:          testKeyID,
			Identifiers: []*PublicIdentifier{}, // Empty identifiers array
		}

		return 200, response
	})
	defer done()

	signingModule := newTestSigningModule(t)
	configureTestSigningModule(t, ctx, signingModule, httpEndpoint)

	_, err := signingModule.ResolveKey(ctx, &prototk.ResolveKeyRequest{
		Name:                testKeyName,
		Path:                testPathSegments,
		RequiredIdentifiers: testRequiredIdentifiers,
	})
	require.Regexp(t, "KA180006", err)
}

func TestResolveKeyNotFound(t *testing.T) {
	ctx, httpEndpoint, done := newTestServer(t, func(r *http.Request) (int, interface{}) {
		validateResolveKeyRequest(t, r)
		return 404, createErrorResponse("key not found")
	})
	defer done()

	signingModule := newTestSigningModule(t)
	configureTestSigningModule(t, ctx, signingModule, httpEndpoint)

	_, err := signingModule.ResolveKey(ctx, &prototk.ResolveKeyRequest{
		Name:                testKeyName,
		Path:                testPathSegments,
		RequiredIdentifiers: testRequiredIdentifiers,
	})
	require.Regexp(t, "KA180011", err)                        // MsgKMSKeyNotFound
	require.Regexp(t, "kld:///keystore/parent/key/key1", err) // Should include the URI in the error message
}

func TestResolveKeyHTTPError(t *testing.T) {
	ctx, httpEndpoint, done := newTestServer(t, func(r *http.Request) (int, interface{}) {
		validateResolveKeyRequest(t, r)
		return 500, createErrorResponse("internal server error")
	})
	defer done()

	signingModule := newTestSigningModule(t)
	configureTestSigningModule(t, ctx, signingModule, httpEndpoint)

	_, err := signingModule.ResolveKey(ctx, &prototk.ResolveKeyRequest{
		Name:                testKeyName,
		Path:                testPathSegments,
		RequiredIdentifiers: testRequiredIdentifiers,
	})
	require.Regexp(t, "KA180010", err) // MsgKMSAPICallFailed
	require.Regexp(t, "internal server error", err)
}

func TestResolveKeyNetworkError(t *testing.T) {
	ctx := context.Background()
	signingModule := newTestSigningModule(t)
	configureTestSigningModule(t, ctx, signingModule, "http://invalid-endpoint-that-does-not-exist:9999")

	_, err := signingModule.ResolveKey(ctx, &prototk.ResolveKeyRequest{
		Name:                testKeyName,
		Path:                testPathSegments,
		RequiredIdentifiers: testRequiredIdentifiers,
	})
	require.Regexp(t, "KA180010", err) // MsgKMSAPICallFailed
}

func TestResolveKeyHTTPErrorNoErrorBody(t *testing.T) {
	ctx, httpEndpoint, done := newTestServer(t, func(r *http.Request) (int, interface{}) {
		validateResolveKeyRequest(t, r)
		return 500, nil
	})
	defer done()

	signingModule := newTestSigningModule(t)
	configureTestSigningModule(t, ctx, signingModule, httpEndpoint)

	_, err := signingModule.ResolveKey(ctx, &prototk.ResolveKeyRequest{
		Name:                testKeyName,
		Path:                testPathSegments,
		RequiredIdentifiers: testRequiredIdentifiers,
	})
	require.Regexp(t, "KA180010", err) // MsgKMSAPICallFailed
}

func TestResolveKeyWithAutoKeyCreation(t *testing.T) {
	ctx, httpEndpoint, done := newTestServer(t, func(r *http.Request) (int, interface{}) {
		resolveReq := validateResolveKeyRequest(t, r)

		// Verify autoKeyCreation and KeySpec are set correctly
		assert.True(t, resolveReq.AutoKeyCreation)
		assert.Equal(t, EC_NIST_P256, resolveReq.KeySpec)

		return 200, createResolveKeyResponse(testKeyID, testEthAddress)
	})
	defer done()

	signingModule := newTestSigningModule(t)
	configureTestSigningModule(t, ctx, signingModule, httpEndpoint)

	res, err := signingModule.ResolveKey(ctx, &prototk.ResolveKeyRequest{
		Name:                testKeyName,
		Path:                testPathSegments,
		RequiredIdentifiers: testRequiredIdentifiers,
	})
	require.NoError(t, err)
	assert.Equal(t, testKeyID, res.KeyHandle)
}

func TestResolveKeyWithAutoPublicIdentifiersCreation(t *testing.T) {
	ctx, httpEndpoint, done := newTestServer(t, func(r *http.Request) (int, interface{}) {
		switch r.URL.Path {
		case "/resolve":
			if r.Body != nil {
				var resolveReq ResolveKeyRequest
				err := json.NewDecoder(r.Body).Decode(&resolveReq)
				assert.NoError(t, err)

				// Verify that AutoPublicIdentifiersCreation is set to true
				assert.True(t, resolveReq.AutoPublicIdentifiersCreation)

				// Return successful response with key ID and identifiers
				return 200, &ResolveKeyResponse{
					ID: testKeyID,
					Identifiers: []*PublicIdentifier{
						{
							Type:  ADDRESS_ETH,
							Value: testEthAddress,
						},
					},
				}
			}
		}

		return 404, createErrorResponse("not found")
	})
	defer done()

	signingModule := newTestSigningModule(t)
	configureTestSigningModule(t, ctx, signingModule, httpEndpoint)

	res, err := signingModule.ResolveKey(ctx, &prototk.ResolveKeyRequest{
		Name:                testKeyName,
		Path:                testPathSegments,
		RequiredIdentifiers: testRequiredIdentifiers,
	})
	require.NoError(t, err)
	assert.Equal(t, testKeyID, res.KeyHandle)
	assert.Len(t, res.Identifiers, 1)
	assert.Equal(t, algorithms.ECDSA_SECP256K1, res.Identifiers[0].Algorithm)
	assert.Equal(t, verifiers.ETH_ADDRESS, res.Identifiers[0].VerifierType)
	assert.Equal(t, testEthAddress, res.Identifiers[0].Verifier)
}

func TestSignOkECDSA_SECP256K1(t *testing.T) {
	// Use deterministic bytes instead of random bytes for consistent test results
	payloadRespBase64 := "72ujCh4Eg8gl4+PpjQ9aAZovHshn5X1yv1j0t2jCTzg3GswBSu1c/6NsWCxGIJTXaGiXYGUK2kIODSRbDu+3mgE="
	expectedPayloadBytes, err := base64.StdEncoding.DecodeString(payloadRespBase64)
	require.NoError(t, err)

	ctx, httpEndpoint, done := newTestServer(t, func(r *http.Request) (int, interface{}) {
		validateSignRequest(t, r)
		return 200, createSignResponse(payloadRespBase64)
	})
	defer done()

	signingModule := newTestSigningModule(t)
	configureTestSigningModule(t, ctx, signingModule, httpEndpoint)

	res, err := signingModule.Sign(ctx, testSignRequest)
	require.NoError(t, err)
	assert.NotEmpty(t, res.Payload)
	assert.Equal(t, expectedPayloadBytes, res.Payload)
}

func TestSignInvalidPayloadTypeError(t *testing.T) {
	ctx := context.Background()
	signingModule := newTestSigningModule(t)
	configureTestSigningModule(t, ctx, signingModule, "http://testendpoint")

	_, err := signingModule.Sign(ctx, &prototk.SignWithKeyRequest{
		PayloadType: "NotSupported",
	})
	require.Regexp(t, "KA180007", err)
}

func TestSignEmptyPayloadError(t *testing.T) {
	ctx := context.Background()
	signingModule := newTestSigningModule(t)
	configureTestSigningModule(t, ctx, signingModule, "http://testendpoint")

	_, err := signingModule.Sign(ctx, &prototk.SignWithKeyRequest{
		PayloadType: signpayloads.OPAQUE_TO_RSV,
	})
	require.Regexp(t, "KA180008", err)
}

func TestSignSignError(t *testing.T) {
	ctx, httpEndpoint, done := newTestServer(t, func(r *http.Request) (int, interface{}) {
		validateSignRequest(t, r)
		return 500, createErrorResponse("sign error")
	})
	defer done()

	signingModule := newTestSigningModule(t)
	configureTestSigningModule(t, ctx, signingModule, httpEndpoint)

	_, err := signingModule.Sign(ctx, testSignRequest)
	require.Regexp(t, "KA180010", err)
	require.Regexp(t, "sign error", err)
}

func TestSignInvalidSignatureLength(t *testing.T) {
	ctx, httpEndpoint, done := newTestServer(t, func(r *http.Request) (int, interface{}) {
		validateSignRequest(t, r)

		// Return a successful response with 64-byte payload (invalid length for ECDSA_SECP256K1)
		response := &SignResponse{
			Payload: "ESIzRFVmd4iZqru8zd7v/wECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8gISIjJCUmJygpKissLS4vMDEyMzQ1Njc4OTo7PD0+P0Ah",
		}

		return 200, response
	})
	defer done()

	signingModule := newTestSigningModule(t)
	configureTestSigningModule(t, ctx, signingModule, httpEndpoint)

	_, err := signingModule.Sign(ctx, testSignRequest)
	require.Regexp(t, "KA180009", err)
}

func TestSignKeyNotFound(t *testing.T) {
	ctx, httpEndpoint, done := newTestServer(t, func(r *http.Request) (int, interface{}) {
		validateSignRequest(t, r)
		return 404, createErrorResponse("key not found")
	})
	defer done()

	signingModule := newTestSigningModule(t)
	configureTestSigningModule(t, ctx, signingModule, httpEndpoint)

	_, err := signingModule.Sign(ctx, testSignRequest)
	require.Regexp(t, "KA180011", err) // MsgKMSKeyNotFound
}

func TestSignInvalidBase64Response(t *testing.T) {
	ctx, httpEndpoint, done := newTestServer(t, func(r *http.Request) (int, interface{}) {
		validateSignRequest(t, r)
		return 200, createSignResponse("invalid-base64-string!")
	})
	defer done()

	signingModule := newTestSigningModule(t)
	configureTestSigningModule(t, ctx, signingModule, httpEndpoint)

	_, err := signingModule.Sign(ctx, testSignRequest)
	require.Regexp(t, "KA180012", err) // MsgKMSInvalidBase64Response
	require.Regexp(t, "invalid-base64-string!", err)
}

func TestSignNetworkError(t *testing.T) {
	ctx := context.Background()
	signingModule := newTestSigningModule(t)
	configureTestSigningModule(t, ctx, signingModule, "http://invalid-endpoint-that-does-not-exist:9999")

	_, err := signingModule.Sign(ctx, testSignRequest)
	require.Regexp(t, "KA180010", err) // MsgKMSAPICallFailed
}

func TestSignHTTPErrorNoErrorBody(t *testing.T) {
	ctx, httpEndpoint, done := newTestServer(t, func(r *http.Request) (int, interface{}) {
		validateSignRequest(t, r)
		return 500, nil
	})
	defer done()

	signingModule := newTestSigningModule(t)
	configureTestSigningModule(t, ctx, signingModule, httpEndpoint)

	_, err := signingModule.Sign(ctx, testSignRequest)
	require.Regexp(t, "KA180010", err) // MsgKMSAPICallFailed
}

func TestSignUnsupportedAlgorithmError(t *testing.T) {
	ctx := context.Background()
	signingModule := newTestSigningModule(t)
	configureTestSigningModule(t, ctx, signingModule, "http://testendpoint")

	_, err := signingModule.Sign(ctx, &prototk.SignWithKeyRequest{
		KeyHandle:   testKeyID,
		Algorithm:   "some_other_algorithm",
		PayloadType: signpayloads.OPAQUE_TO_RSV,
		Payload:     []byte(testPayload),
	})
	require.Regexp(t, "KA180013", err)
	require.Regexp(t, "some_other_algorithm", err)
	require.Regexp(t, algorithms.ECDSA_SECP256K1, err)
}

func TestListKeysOk(t *testing.T) {
	ctx := context.Background()
	signingModule := newTestSigningModule(t)
	configureTestSigningModule(t, ctx, signingModule, "http://testendpoint")

	_, err := signingModule.ListKeys(ctx, &prototk.ListKeysRequest{})
	require.NoError(t, err)
}

func TestCloseUnsupportedError(t *testing.T) {
	signingModule := newTestSigningModule(t)
	configureTestSigningModuleWithBgCtx(t, signingModule, "http://testendpoint")

	_, err := signingModule.Close(signingModule.bgCtx, &prototk.CloseRequest{})
	require.NoError(t, err)
}

func TestGetURIWithKeystoreName(t *testing.T) {
	signingModule := newTestSigningModule(t)

	// Configure with keystoreName
	_, err := signingModule.ConfigureSigningModule(signingModule.bgCtx, &prototk.ConfigureSigningModuleRequest{
		Name: "test",
		ConfigJson: `{
			"keystoreName": "mykeystore",
			"httpConfig": {
				"url": "http://testendpoint"
			}
		}`,
	})
	require.NoError(t, err)

	// Test with keystoreName set - should use keystoreName and all path segments
	uri, err := signingModule.GetURI(signingModule.bgCtx, "mykey", []*prototk.ResolveKeyPathSegment{
		{Name: "folder1", Index: 0},
		{Name: "folder2", Index: 0},
		{Name: "folder3", Index: 0},
	})
	require.NoError(t, err)
	assert.Equal(t, "kld:///keystore/mykeystore/key/folder1/folder2/folder3/mykey", uri)
}

func TestGetURIWithoutKeystoreName(t *testing.T) {
	signingModule := newTestSigningModule(t)

	// Configure without keystoreName
	_, err := signingModule.ConfigureSigningModule(signingModule.bgCtx, &prototk.ConfigureSigningModuleRequest{
		Name: "test",
		ConfigJson: `{
			"httpConfig": {
				"url": "http://testendpoint"
			}
		}`,
	})
	require.NoError(t, err)

	// Test without keystoreName - first path segment becomes keystore name
	uri, err := signingModule.GetURI(signingModule.bgCtx, "mykey", []*prototk.ResolveKeyPathSegment{
		{Name: "keystore1", Index: 0},
		{Name: "folder1", Index: 0},
		{Name: "folder2", Index: 0},
	})
	require.NoError(t, err)
	assert.Equal(t, "kld:///keystore/keystore1/key/folder1/folder2/mykey", uri)
}

func TestGetURIWithSinglePathSegment(t *testing.T) {
	signingModule := newTestSigningModule(t)

	// Configure without keystoreName
	_, err := signingModule.ConfigureSigningModule(signingModule.bgCtx, &prototk.ConfigureSigningModuleRequest{
		Name: "test",
		ConfigJson: `{
			"httpConfig": {
				"url": "http://testendpoint"
			}
		}`,
	})
	require.NoError(t, err)

	// Test with single path segment - should become keystore name with no additional path
	uri, err := signingModule.GetURI(signingModule.bgCtx, "mykey", []*prototk.ResolveKeyPathSegment{
		{Name: "keystore1", Index: 0},
	})
	require.NoError(t, err)
	assert.Equal(t, "kld:///keystore/keystore1/key/mykey", uri)
}

func TestGetURIWithNoPathSegments(t *testing.T) {
	signingModule := newTestSigningModule(t)

	// Configure without keystoreName
	_, err := signingModule.ConfigureSigningModule(signingModule.bgCtx, &prototk.ConfigureSigningModuleRequest{
		Name: "test",
		ConfigJson: `{
			"httpConfig": {
				"url": "http://testendpoint"
			}
		}`,
	})
	require.NoError(t, err)

	// Test with no path segments - should return error
	_, err = signingModule.GetURI(signingModule.bgCtx, "mykey", []*prototk.ResolveKeyPathSegment{})
	require.Error(t, err)
	require.Regexp(t, "KA180005", err) // MsgResolvePaladinTypeMissingPaths
}

func TestGetURIWithKeystoreNameAndNoPathSegments(t *testing.T) {
	signingModule := newTestSigningModule(t)

	// Configure with keystoreName
	_, err := signingModule.ConfigureSigningModule(signingModule.bgCtx, &prototk.ConfigureSigningModuleRequest{
		Name: "test",
		ConfigJson: `{
			"keystoreName": "mykeystore",
			"httpConfig": {
				"url": "http://testendpoint"
			}
		}`,
	})
	require.NoError(t, err)

	// Test with keystoreName but no path segments - should work fine
	uri, err := signingModule.GetURI(signingModule.bgCtx, "mykey", []*prototk.ResolveKeyPathSegment{})
	require.NoError(t, err)
	assert.Equal(t, "kld:///keystore/mykeystore/key/mykey", uri)
}

func TestGetURIWithComplexPathStructure(t *testing.T) {
	signingModule := newTestSigningModule(t)

	// Configure with keystoreName
	_, err := signingModule.ConfigureSigningModule(signingModule.bgCtx, &prototk.ConfigureSigningModuleRequest{
		Name: "test",
		ConfigJson: `{
			"keystoreName": "enterprise-keystore",
			"httpConfig": {
				"url": "http://testendpoint"
			}
		}`,
	})
	require.NoError(t, err)

	// Test with complex nested path structure
	uri, err := signingModule.GetURI(signingModule.bgCtx, "signing-key", []*prototk.ResolveKeyPathSegment{
		{Name: "accounts", Index: 0},
		{Name: "department", Index: 0},
		{Name: "finance", Index: 0},
		{Name: "keys", Index: 0},
		{Name: "production", Index: 0},
	})
	require.NoError(t, err)
	assert.Equal(t, "kld:///keystore/enterprise-keystore/key/accounts/department/finance/keys/production/signing-key", uri)
}

func TestGetURIWithEmptyKeyName(t *testing.T) {
	signingModule := newTestSigningModule(t)

	// Configure with keystoreName
	_, err := signingModule.ConfigureSigningModule(signingModule.bgCtx, &prototk.ConfigureSigningModuleRequest{
		Name: "test",
		ConfigJson: `{
			"keystoreName": "mykeystore",
			"httpConfig": {
				"url": "http://testendpoint"
			}
		}`,
	})
	require.NoError(t, err)

	// Test with empty key name - should still work
	uri, err := signingModule.GetURI(signingModule.bgCtx, "", []*prototk.ResolveKeyPathSegment{
		{Name: "folder1", Index: 0},
	})
	require.NoError(t, err)
	assert.Equal(t, "kld:///keystore/mykeystore/key/folder1/", uri)
}

func TestGetURIWithFolderPath(t *testing.T) {
	signingModule := newTestSigningModule(t)

	// Configure with folderPath
	_, err := signingModule.ConfigureSigningModule(signingModule.bgCtx, &prototk.ConfigureSigningModuleRequest{
		Name: "test",
		ConfigJson: `{
			"keystoreName": "mykeystore",
			"folderPath": "base/path",
			"httpConfig": {
				"url": "http://testendpoint"
			}
		}`,
	})
	require.NoError(t, err)

	// Test with folderPath - should prepend folderPath to key path
	uri, err := signingModule.GetURI(signingModule.bgCtx, "mykey", []*prototk.ResolveKeyPathSegment{
		{Name: "folder1", Index: 0},
		{Name: "folder2", Index: 0},
	})
	require.NoError(t, err)
	assert.Equal(t, "kld:///keystore/mykeystore/key/base/path/folder1/folder2/mykey", uri)
}

func TestGetURIWithFolderPathWithoutKeystoreName(t *testing.T) {
	signingModule := newTestSigningModule(t)

	// Configure with folderPath but without keystoreName - should fail validation
	_, err := signingModule.ConfigureSigningModule(signingModule.bgCtx, &prototk.ConfigureSigningModuleRequest{
		Name: "test",
		ConfigJson: `{
			"folderPath": "base/path",
			"httpConfig": {
				"url": "http://testendpoint"
			}
		}`,
	})
	require.Error(t, err)
	require.Regexp(t, "KA180014", err) // MsgFolderPathRequiresKeystoreName
}

func TestGetURIWithFolderPathAndNoPathSegments(t *testing.T) {
	signingModule := newTestSigningModule(t)

	// Configure with folderPath and keystoreName
	_, err := signingModule.ConfigureSigningModule(signingModule.bgCtx, &prototk.ConfigureSigningModuleRequest{
		Name: "test",
		ConfigJson: `{
			"keystoreName": "mykeystore",
			"folderPath": "base/path",
			"httpConfig": {
				"url": "http://testendpoint"
			}
		}`,
	})
	require.NoError(t, err)

	// Test with folderPath but no additional path segments
	uri, err := signingModule.GetURI(signingModule.bgCtx, "mykey", []*prototk.ResolveKeyPathSegment{})
	require.NoError(t, err)
	assert.Equal(t, "kld:///keystore/mykeystore/key/base/path/mykey", uri)
}

func TestGetURIWithEmptyFolderPath(t *testing.T) {
	signingModule := newTestSigningModule(t)

	// Configure with empty folderPath
	_, err := signingModule.ConfigureSigningModule(signingModule.bgCtx, &prototk.ConfigureSigningModuleRequest{
		Name: "test",
		ConfigJson: `{
			"keystoreName": "mykeystore",
			"folderPath": "",
			"httpConfig": {
				"url": "http://testendpoint"
			}
		}`,
	})
	require.NoError(t, err)

	// Test with empty folderPath - should behave like no folderPath
	uri, err := signingModule.GetURI(signingModule.bgCtx, "mykey", []*prototk.ResolveKeyPathSegment{
		{Name: "folder1", Index: 0},
		{Name: "folder2", Index: 0},
	})
	require.NoError(t, err)
	assert.Equal(t, "kld:///keystore/mykeystore/key/folder1/folder2/mykey", uri)
}
