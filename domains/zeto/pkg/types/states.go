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

package types

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/msgs"
	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/zeto/signer/common"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/zetosigner"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/smt"
	"github.com/LFDT-Paladin/smt/pkg/utxo"
	"github.com/LFDT-Paladin/smt/pkg/utxo/core"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/iden3/go-iden3-crypto/poseidon"
)

var ZetoCoinABI = &abi.Parameter{
	Name:         "ZetoCoin",
	Indexed:      true,
	Type:         "tuple",
	InternalType: "struct ZetoCoin",
	Components: abi.ParameterArray{
		{Name: "salt", Type: "uint256"},
		{Name: "owner", Type: "bytes32", Indexed: true},
		{Name: "amount", Type: "uint256"},
		{Name: "locked", Type: "bool", Indexed: true},
	},
}

type ZetoCoinState struct {
	ID              pldtypes.HexUint256 `json:"id"`
	Created         pldtypes.Timestamp  `json:"created"`
	ContractAddress pldtypes.EthAddress `json:"contractAddress"`
	Data            ZetoCoin            `json:"data"`
}

type ZetoCoin struct {
	Salt   *pldtypes.HexUint256 `json:"salt"`
	Owner  pldtypes.HexBytes    `json:"owner"`
	Amount *pldtypes.HexUint256 `json:"amount"`
	Locked bool                 `json:"locked"`
	hash   *pldtypes.HexUint256
}

func (z *ZetoCoin) Hash(ctx context.Context) (*pldtypes.HexUint256, error) {
	if z.hash == nil {
		ownerKey, err := zetosigner.DecodeBabyJubJubPublicKey(z.Owner.HexString())
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorDecodeBJJKey, err)
		}
		commitment, err := poseidon.Hash([]*big.Int{
			z.Amount.Int(),
			z.Salt.Int(),
			ownerKey.X,
			ownerKey.Y,
		})
		if err != nil {
			return nil, err
		}
		z.hash = (*pldtypes.HexUint256)(commitment)
	}
	return z.hash, nil
}

var ZetoNFTokenABI = &abi.Parameter{
	Name:         "ZetoNFToken",
	Indexed:      true,
	Type:         "tuple",
	InternalType: "struct ZetoNFToken",
	Components: abi.ParameterArray{
		{Name: "salt", Type: "uint256"},
		{Name: "uri", Type: "string"},
		{Name: "owner", Type: "bytes32", Indexed: true},
		{Name: "tokenID", Type: "uint256", Indexed: true},
	},
}

type TransactionData struct {
	Salt *pldtypes.HexUint256 `json:"salt"`
	Data pldtypes.HexBytes    `json:"data"`
	hash *pldtypes.HexUint256
}

var TransactionDataABI = &abi.Parameter{
	Name:         "TransactionData",
	Type:         "tuple",
	InternalType: "struct TransactionData",
	Components: abi.ParameterArray{
		{Name: "salt", Type: "bytes32"},
		{Name: "data", Type: "bytes"},
	},
}

func (z *TransactionData) Hash(ctx context.Context) (*pldtypes.HexUint256, error) {
	if z.hash == nil {
		hash := sha256.New()
		hash.Write(z.Salt.Int().Bytes())
		hash.Write(z.Data)
		hashBytes := pldtypes.HexBytes(hash.Sum(nil))
		hashInt, err := pldtypes.ParseHexUint256(ctx, hashBytes.String())
		if err != nil {
			return nil, err
		}
		z.hash = hashInt
	}
	return z.hash, nil
}

// ZetoNFTState represents the overall state of an NFT.
type ZetoNFTState struct {
	ID              pldtypes.HexUint256 `json:"id"`
	Created         pldtypes.Timestamp  `json:"created"`
	ContractAddress pldtypes.EthAddress `json:"contractAddress"`
	Data            ZetoNFToken         `json:"data"`
}

// ZetoNFToken holds the NFT token details.
type ZetoNFToken struct {
	Salt      *pldtypes.HexUint256 `json:"salt"`
	URI       string               `json:"uri"`
	Owner     pldtypes.HexBytes    `json:"owner"`
	TokenID   *pldtypes.HexUint256 `json:"tokenID"`
	utxoToken core.UTXO            // Calculated from TokenID, URI, etc.
}

// NewZetoNFToken creates a new ZetoNFToken from the given parameters.
func NewZetoNFToken(tokenID *pldtypes.HexUint256, uri string, publicKey *babyjub.PublicKey, salt *big.Int) *ZetoNFToken {
	return &ZetoNFToken{
		Salt:    (*pldtypes.HexUint256)(salt),
		URI:     uri,
		Owner:   pldtypes.MustParseHexBytes(zetosigner.EncodeBabyJubJubPublicKey(publicKey)),
		TokenID: tokenID,
	}
}

// UnmarshalJSON implements the json.Unmarshaler interface for ZetoNFToken.
func (z *ZetoNFToken) UnmarshalJSON(data []byte) error {
	type alias ZetoNFToken // alias to avoid infinite recursion during unmarshaling

	var tmp alias
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	*z = ZetoNFToken(tmp)
	return nil
}

// Hash calculates the hash of the token using its UTXO representation.
func (z *ZetoNFToken) Hash(ctx context.Context) (*pldtypes.HexUint256, error) {
	if z.utxoToken == nil {
		if err := z.setUTXO(); err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgInvalidUTXO, err)
		}
	}
	hash, err := z.utxoToken.GetHash()
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorHashState, err)
	}
	return (*pldtypes.HexUint256)(hash), nil
}

// setUTXO validates required fields and calculates the UTXO token.
func (z *ZetoNFToken) setUTXO() error {
	if err := z.validate(); err != nil {
		return err
	}

	// Decode the public key from the Owner field.
	publicKey, err := zetosigner.DecodeBabyJubJubPublicKey(z.Owner.HexString())
	if err != nil {
		return err
	}

	z.utxoToken = utxo.NewNonFungible(z.TokenID.Int(), z.URI, publicKey, z.Salt.Int(), common.GetHasher())
	return nil
}

// validate ensures all required fields are present.
func (z *ZetoNFToken) validate() error {
	if z.TokenID == nil {
		return fmt.Errorf("tokenID is missing")
	}
	if z.URI == "" {
		return fmt.Errorf("uri is empty")
	}
	if z.Owner == nil {
		return fmt.Errorf("owner is missing")
	}
	if z.Salt == nil {
		return fmt.Errorf("salt is missing")
	}
	return nil
}

func GetStateSchemas() ([]string, error) {
	coinJSON, _ := json.Marshal(ZetoCoinABI)
	nftJSON, _ := json.Marshal(ZetoNFTokenABI)
	smtRootJSON, _ := json.Marshal(smt.MerkleTreeRootABI)
	smtNodeJSON, _ := json.Marshal(smt.MerkleTreeNodeABI)
	infoJSON, _ := json.Marshal(TransactionDataABI)

	return []string{
		string(coinJSON),
		string(nftJSON),
		string(smtRootJSON),
		string(smtNodeJSON),
		string(infoJSON),
	}, nil
}
