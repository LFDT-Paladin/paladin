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

	"github.com/go-resty/resty/v2"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldresty"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/algorithms"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/plugintk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/signpayloads"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/verifiers"
	"github.com/kaleido-io/key-manager/signingmodules/kaleidokms/internal/msgs"
)

type Server interface {
	Start() error
	Stop()
}

type keyManagerSigningModule struct {
	bgCtx     context.Context
	callbacks plugintk.SigningModuleCallbacks

	conf       *Config
	name       string
	httpClient *resty.Client

	keystoreName string
	folderPath   string
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func NewPlugin(ctx context.Context) plugintk.PluginBase {
	return plugintk.NewSigningModule(NewKeyManagerSigningModule)
}

func NewKeyManagerSigningModule(callbacks plugintk.SigningModuleCallbacks) plugintk.SigningModuleAPI {
	return &keyManagerSigningModule{
		bgCtx:     context.Background(),
		callbacks: callbacks,
	}
}

func (rsm *keyManagerSigningModule) ConfigureSigningModule(ctx context.Context, req *prototk.ConfigureSigningModuleRequest) (*prototk.ConfigureSigningModuleResponse, error) {
	rsm.name = req.Name

	err := json.Unmarshal([]byte(req.ConfigJson), &rsm.conf)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgInvalidSigningModuleConfig)
	}

	// Set keystoreName from config if provided
	rsm.keystoreName = rsm.conf.KeystoreName

	// Set folderPath from config if provided
	rsm.folderPath = rsm.conf.FolderPath

	// Validate that if folderPath is set, keystoreName is also provided
	if rsm.folderPath != "" && rsm.keystoreName == "" {
		return nil, i18n.NewError(ctx, msgs.MsgFolderPathRequiresKeystoreName)
	}

	if rsm.conf.HTTPConfig != nil {
		httpClient, err := pldresty.New(ctx, rsm.conf.HTTPConfig)
		if err != nil {
			return nil, err
		}

		rsm.httpClient = httpClient
	}

	return &prototk.ConfigureSigningModuleResponse{}, nil
}

func (rsm *keyManagerSigningModule) GetURI(ctx context.Context, keyName string, keyPaths []*prototk.ResolveKeyPathSegment) (string, error) {
	path := keyPaths
	var keystore string
	if rsm.keystoreName != "" {
		keystore = rsm.keystoreName
	} else {
		// The first key path is used as the "keystore/signer" name to where the key will be resolved
		if len(keyPaths) == 0 {
			return "", i18n.NewError(ctx, msgs.MsgResolvePaladinTypeMissingPaths)
		}
		keystore = keyPaths[0].Name // use the first segment as the keystore name
		path = path[1:]             // remove the first segment
	}

	var keyPath string

	// Prepend folder path if configured
	if rsm.folderPath != "" {
		keyPath = fmt.Sprintf("%s/", rsm.folderPath)
	}

	for _, segment := range path {
		keyPath = fmt.Sprintf("%s%s/", keyPath, segment.Name)
	}

	return fmt.Sprintf("kld:///keystore/%s/key/%s%s", keystore, keyPath, keyName), nil
}

func (rsm *keyManagerSigningModule) ResolveKey(ctx context.Context, req *prototk.ResolveKeyRequest) (*prototk.ResolveKeyResponse, error) {
	if len(req.Name) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgResolveKeyCannotBeEmpty)
	}

	if len(req.RequiredIdentifiers) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgResolveKeyMissingIdentifiers)
	}

	if (req.RequiredIdentifiers[0].VerifierType != verifiers.ETH_ADDRESS) || (req.RequiredIdentifiers[0].Algorithm != algorithms.ECDSA_SECP256K1) {
		return nil, i18n.NewError(ctx, msgs.MsgResolveKeyInvalidIdentifierType)
	}

	uri, err := rsm.GetURI(ctx, req.Name, req.Path)
	if err != nil {
		return nil, err
	}

	log.L(ctx).Debugf("KeyManagerSigningModule ResolveKey uri: %s ", uri)

	keyManagerResolveRequest := ResolveKeyRequest{
		KeyIdentifier: &KeyIdentifier{
			KeyURI: &uri,
		},
		PublicIdentifierTypesToResolve: []string{
			ADDRESS_ETH,
		},
		AutoKeyCreation:               true,
		AutoPublicIdentifiersCreation: true,
		KeySpec:                       EC_NIST_P256,
	}

	var response ResolveKeyResponse
	var errInfo ErrorResponse

	res, err := rsm.httpClient.R().
		SetContext(ctx).
		SetBody(keyManagerResolveRequest).
		SetResult(&response).
		SetError(&errInfo).
		Post("/resolve")

	if err != nil || (!res.IsSuccess() && res.StatusCode() != http.StatusNotFound) {
		return nil, rsm.kmsRequestError(ctx, err, errInfo)
	}

	if res.StatusCode() == http.StatusNotFound {
		return nil, i18n.NewError(ctx, msgs.MsgKMSKeyNotFound, uri)
	}

	if len(response.Identifiers) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgResolveKeyNoIdentifiersResponse, *keyManagerResolveRequest.KeyIdentifier.KeyURI)
	}

	return &prototk.ResolveKeyResponse{
		KeyHandle: response.ID,
		Identifiers: []*prototk.PublicKeyIdentifier{
			{
				Algorithm:    algorithms.ECDSA_SECP256K1,
				VerifierType: verifiers.ETH_ADDRESS,
				Verifier:     response.Identifiers[0].Value,
			},
		}}, nil
}

func (rsm *keyManagerSigningModule) Sign(ctx context.Context, req *prototk.SignWithKeyRequest) (*prototk.SignWithKeyResponse, error) {
	if req.PayloadType != signpayloads.OPAQUE_TO_RSV {
		return nil, i18n.NewError(ctx, msgs.MsgSignInvalidPayloadType, req.PayloadType)
	}

	if len(req.Payload) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgSignPayloadCannotBeEmpty)
	}

	if req.Algorithm != algorithms.ECDSA_SECP256K1 {
		return nil, i18n.NewError(ctx, msgs.MsgSignUnsupportedAlgorithm, req.Algorithm, algorithms.ECDSA_SECP256K1)
	}

	signRequest := SignRequest{
		KeyIdentifier: &KeyIdentifier{
			KeyID: &req.KeyHandle,
		},
		Payload:          base64.StdEncoding.EncodeToString(req.Payload),
		SigningAlgorithm: ECDSA_256,
		SigningFormat:    DIGEST_TO_RSV,
	}

	var response SignResponse
	var errInfo ErrorResponse

	res, err := rsm.httpClient.R().
		SetContext(ctx).
		SetBody(signRequest).
		SetResult(&response).
		SetError(&errInfo).
		Post("/sign")

	if err != nil || (!res.IsSuccess() && res.StatusCode() != http.StatusNotFound) {
		return nil, rsm.kmsRequestError(ctx, err, errInfo)
	}

	if res.StatusCode() == http.StatusNotFound {
		return nil, i18n.NewError(ctx, msgs.MsgKMSKeyNotFound, req.KeyHandle)
	}

	payloadBytes, err := base64.StdEncoding.DecodeString(response.Payload)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgKMSInvalidBase64Response, response.Payload)
	}

	if len(payloadBytes) != 65 {
		return nil, i18n.NewError(ctx, msgs.MsgSignInvalidSignatureLength, len(payloadBytes))
	}

	return &prototk.SignWithKeyResponse{
		Payload: payloadBytes,
	}, nil
}

func (rsm *keyManagerSigningModule) ListKeys(ctx context.Context, req *prototk.ListKeysRequest) (*prototk.ListKeysResponse, error) {
	return &prototk.ListKeysResponse{
		Items: []*prototk.ListKeyEntry{},
	}, nil
}

func (rsm *keyManagerSigningModule) Close(ctx context.Context, req *prototk.CloseRequest) (*prototk.CloseResponse, error) {
	return &prototk.CloseResponse{}, nil
}

func (rsm *keyManagerSigningModule) kmsRequestError(ctx context.Context, maybeErr error, maybeErrInfo ErrorResponse) error {
	var infoText string
	if maybeErrInfo.Error != "" {
		infoText = maybeErrInfo.Error
	} else if maybeErr != nil {
		infoText = maybeErr.Error()
	}
	return i18n.WrapError(ctx, maybeErr, msgs.MsgKMSAPICallFailed, infoText)
}
