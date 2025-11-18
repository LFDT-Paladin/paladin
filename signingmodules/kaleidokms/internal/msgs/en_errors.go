// Copyright © 2025 Kaleido, Inc.
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

package msgs

import (
	"sync"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"golang.org/x/text/language"
)

var registered sync.Once
var pde = func(key, translation string, statusHint ...int) i18n.ErrorMessageKey {
	registered.Do(func() {
		i18n.RegisterPrefix("KA18", "Kaleido - Paladin Signing Module")
	})
	return i18n.PDE(language.AmericanEnglish, key, translation, statusHint...)
}

var (
	MsgInvalidSigningModuleConfig      = pde("KA180001", "Invalid signing module configuration")
	MsgResolveKeyCannotBeEmpty         = pde("KA180002", "Cannot resolve a key with no name")
	MsgResolveKeyMissingIdentifiers    = pde("KA180003", "Resolve key request must contain at least one public identifier")
	MsgResolveKeyInvalidIdentifierType = pde("KA180004", "Resolving a key with a required identifier type '%s' is not supported")
	MsgResolvePaladinTypeMissingPaths  = pde("KA180005", "Error resolving key: no key store name in path")
	MsgResolveKeyNoIdentifiersResponse = pde("KA180006", "No identifiers in response from key manager when resolving key '%s'")
	MsgSignInvalidPayloadType          = pde("KA180007", "Sign with payload type '%s' is not supported")
	MsgSignPayloadCannotBeEmpty        = pde("KA180008", "Cannot sign an empty payload")
	MsgSignInvalidSignatureLength      = pde("KA180009", "Sign signature response length '%d' not equal to 65 bytes")
	MsgKMSAPICallFailed                = pde("KA180010", "KMS API called failed: %s")
	MsgKMSKeyNotFound                  = pde("KA180011", "KMS key with URI '%s' not found")
	MsgKMSInvalidBase64Response        = pde("KA180012", "KMS response contains invalid base64: %s")
	MsgSignUnsupportedAlgorithm        = pde("KA180013", "Sign with algorithm '%s' is not supported, only '%s' is supported")
	MsgFolderPathRequiresKeystoreName  = pde("KA180014", "Folder path configuration requires keystore name to be provided")
)
