/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	_ "embed"

	"github.com/google/uuid"
	"github.com/hyperledger-firefly/signer/pkg/abi"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	zetotypes "github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldclient"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/solutils"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/algorithms"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/verifiers"
)

//go:embed abis/zeto/Zeto_Anon.json
var zetoAnonBuildJSON []byte

// Pools here are deployed on the V1 transaction API against zeto-contracts v0.5.1 implementations, so this is the
// generation carrying createLock/spendLock (V0's lock/transferLocked no longer exist). See ZetoFungibleABIVersion
// and ZetoReleaseGeneration in domains/zeto/pkg/types/versions.go.
var zetoFungibleABI = zetotypes.ZetoFungibleABIForVersion(zetotypes.ZetoFungibleABI_V1)

// zetoLockInfoStateResult mirrors zetotypes.ZetoCoinState for the lock-info schema, which is where createLock
// persists the lock id this suite needs for delegateLock / spendLock.
type zetoLockInfoStateResult struct {
	ID              pldtypes.HexUint256         `json:"id"`
	ContractAddress pldtypes.EthAddress         `json:"contractAddress"`
	Data            zetotypes.ZetoLockInfoState `json:"data"`
}

// zetoDelegateLockArgsABI matches IZetoLockableCapability.ZetoDelegateLockArgs — the ABI-encoded delegateArgs
// argument of delegateLock(bytes32,bytes,address,bytes) on zeto-contracts ~v0.5.x.
var zetoDelegateLockArgsABI = abi.ParameterArray{
	{
		Type:         "tuple",
		InternalType: "struct ZetoDelegateLockArgs",
		Components: abi.ParameterArray{
			{Name: "txId", Type: "bytes32"},
		},
	},
}

const tokenType = "Zeto_Anon"
const isNullifier = false

// const tokenType = "Zeto_AnonNullifier"
// const isNullifier = true

// The V1 axis is opted into at deploy: domainConfigSchema "v1" selects the prefixed on-chain config encoding, and
// zetoVariant / factoryVersion record the Paladin API generation and the upstream release generation respectively.
var zetoConstructorABI = &abi.Entry{
	Type: abi.Constructor, Inputs: abi.ParameterArray{
		{Name: "tokenName", Type: "string"},
		{Name: "domainConfigSchema", Type: "string"},
		{Name: "zetoVariant", Type: "uint256"},
		{Name: "factoryVersion", Type: "uint256"},
	},
}

var _ = Describe(fmt.Sprintf("zeto - %s", tokenType), Ordered, func() {
	BeforeAll(func() {
		// Skip("for now")
	})

	AfterAll(func() {
	})

	Context("Zeto domain verification", func() {

		ctx := context.Background()
		rpc := map[string]pldclient.PaladinClient{}

		connectNode := func(url, name string) {
			Eventually(func() bool {
				return withTimeout(func(ctx context.Context) bool {
					pld, err := pldclient.New().HTTP(ctx, &pldconf.HTTPClientConfig{URL: url})
					if err == nil {
						queriedName, err := pld.Transport().NodeName(ctx)
						Expect(err).To(BeNil())
						Expect(queriedName).To(Equal(name))
						rpc[name] = pld
					}
					return err == nil
				})
			}).Should(BeTrue())
		}

		It("waits to connect to all three nodes", func() {
			connectNode(node1HttpURL, paladinPrefix+"1")
			connectNode(node2HttpURL, paladinPrefix+"2")
			connectNode(node3HttpURL, paladinPrefix+"3")
		})

		It("checks nodes can talk to each other", func() {
			for src := range rpc {
				for dest := range rpc {
					Eventually(func() bool {
						return withTimeout(func(ctx context.Context) bool {
							verifier, err := rpc[src].PTX().ResolveVerifier(ctx, fmt.Sprintf("test@%s", dest),
								algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
							if err == nil {
								addr, err := pldtypes.ParseEthAddress(verifier)
								Expect(err).To(BeNil())
								Expect(addr).ToNot(BeNil())
							}
							return err == nil
						})
					}).Should(BeTrue())
				}
			}
		})

		var zetoContract *pldtypes.EthAddress
		operator := fmt.Sprintf("zeto.operator@%s1", paladinPrefix)
		It("deploys a zeto", func() {
			deploy := rpc[paladinPrefix+"1"].ForABI(ctx, abi.ABI{zetoConstructorABI}).
				Private().
				Domain("zeto").
				Constructor().
				From(operator).
				Inputs(&zetotypes.InitializerParams{
					TokenName:          tokenType,
					DomainConfigSchema: zetotypes.DomainConfigSchemaV1,
					ZetoVariant:        zetotypes.ZetoFungibleABI_V1,
					ReleaseGeneration:  zetotypes.ZetoRelease_V1,
				}).
				Send().
				Wait(5 * time.Second)
			Expect(deploy.Error()).To(BeNil())
			Expect(deploy.Receipt().ContractAddress).ToNot(BeNil())
			zetoContract = deploy.Receipt().ContractAddress
			testLog("Zeto (%s) contract %s deployed by TX %s", tokenType, zetoContract, deploy.ID())
		})

		var zetoLockInfoSchemaID *pldtypes.Bytes32
		var zetoCoinSchemaID *pldtypes.Bytes32
		It("gets the coin and lock-info schemas", func() {
			var schemas []*pldapi.Schema
			err := rpc[paladinPrefix+"1"].CallRPC(ctx, &schemas, "pstate_listSchemas", "zeto")
			Expect(err).To(BeNil())
			for _, s := range schemas {
				if s.Signature == "type=ZetoCoin(uint256 salt,bytes32 owner,uint256 amount,bool locked),labels=[owner,locked]" {
					zetoCoinSchemaID = &s.ID
				}
				// Matched on the type prefix rather than the full signature: the lock-info schema carries a dozen
				// components and pinning all of them here would break on any upstream field addition.
				if strings.HasPrefix(s.Signature, "type=ZetoLockInfoState(") {
					zetoLockInfoSchemaID = &s.ID
				}
			}
			Expect(zetoCoinSchemaID).ToNot(BeNil())
			Expect(zetoLockInfoSchemaID).ToNot(BeNil())
		})

		logWallet := func(identity, node string) {
			var addr pldtypes.HexBytes
			err := rpc[node].CallRPC(ctx, &addr, "ptx_resolveVerifier", identity, "domain:zeto:snark:babyjubjub", "iden3_pubkey_babyjubjub_compressed_0x")
			Expect(err).To(BeNil())
			method := "pstate_queryContractStates"
			if isNullifier {
				method = "pstate_queryContractNullifiers"
			}
			var coins []*zetotypes.ZetoCoinState
			err = rpc[node].CallRPC(ctx, &coins, method, "zeto", zetoContract, zetoCoinSchemaID,
				query.NewQueryBuilder().Equal("owner", addr).Limit(100).Query(),
				"confirmed")
			Expect(err).To(BeNil())
			balance := big.NewInt(0)
			summary := make([]string, len(coins))
			for ic, c := range coins {
				summary[ic] = fmt.Sprintf("%s...[%s]", c.ID.String()[0:8], c.Data.Amount.Int().Text(10))
				balance = new(big.Int).Add(balance, c.Data.Amount.Int())
			}
			testLog("%s@%s balance=%s coins:%v", identity, node, balance, summary)
		}

		It("mints some zetos to bob on node1", func() {
			txn := rpc[paladinPrefix+"1"].ForABI(ctx, zetoFungibleABI).
				Private().
				Domain("zeto").
				Function("mint").
				To(zetoContract).
				From(operator).
				Inputs(&zetotypes.FungibleMintParams{
					Mints: []*zetotypes.FungibleTransferParamEntry{
						{
							To:     fmt.Sprintf("bob@%s1", paladinPrefix),
							Amount: with10Decimals(15),
						},
						{
							To:     fmt.Sprintf("bob@%s1", paladinPrefix),
							Amount: with10Decimals(25),
						},
						{
							To:     fmt.Sprintf("bob@%s1", paladinPrefix),
							Amount: with10Decimals(30),
						},
						{
							To:     fmt.Sprintf("bob@%s1", paladinPrefix),
							Amount: with10Decimals(42),
						},
					},
				}).
				Send().
				Wait(5 * time.Second)
			testLog("Zeto mint transaction %s", txn.ID())
			Expect(txn.Error()).To(BeNil())
			logWallet("bob", paladinPrefix+"1")
		})

		It("sends some zetos to sally on node2", func() {
			for _, amount := range []*pldtypes.HexUint256{
				with10Decimals(33), // 79
				with10Decimals(66), // 13
			} {
				txn := rpc[paladinPrefix+"1"].ForABI(ctx, zetoFungibleABI).
					Private().
					Domain("zeto").
					Function("transfer").
					To(zetoContract).
					From(fmt.Sprintf("bob@%s1", paladinPrefix)).
					Inputs(&zetotypes.FungibleTransferParams{
						Transfers: []*zetotypes.FungibleTransferParamEntry{
							{
								To:     fmt.Sprintf("sally@%s2", paladinPrefix),
								Amount: amount,
							},
						},
					}).
					Send().
					Wait(5 * time.Second)
				testLog("Zeto transfer transaction %s", txn.ID())
				Expect(txn.Error()).To(BeNil())
				logWallet("bob", paladinPrefix+"1")
				logWallet("sally", paladinPrefix+"2")
			}
		})

		It("sally on node2 sends some zetos to fred on node3", func() {
			txn := rpc[paladinPrefix+"2"].ForABI(ctx, zetoFungibleABI).
				Private().
				Domain("zeto").
				Function("transfer").
				To(zetoContract).
				From(fmt.Sprintf("sally@%s2", paladinPrefix)).
				Inputs(&zetotypes.FungibleTransferParams{
					Transfers: []*zetotypes.FungibleTransferParamEntry{
						{
							To:     fmt.Sprintf("fred@%s3", paladinPrefix),
							Amount: with10Decimals(20),
						},
					},
				}).
				Send().
				Wait(5 * time.Second)
			testLog("Zeto transfer transaction %s", txn.ID())
			Expect(txn.Error()).To(BeNil())
			logWallet("sally", paladinPrefix+"2")
			logWallet("fred", paladinPrefix+"3")
			testLog("done testing zeto in isolation")
		})

		It("Bob on node1 creates a lock, delegates it to sally, and sally spends it to fred", func() {
			bobID := fmt.Sprintf("bob@%s1", paladinPrefix)
			sallyID := fmt.Sprintf("sally@%s2", paladinPrefix)
			fredID := fmt.Sprintf("fred@%s3", paladinPrefix)
			sallyEthAddr := getEthAddress(ctx, rpc[paladinPrefix+"2"], "sally", paladinPrefix+"2")
			// ABI only: MustLoadBuild rejects the v0.5.1 build as unlinked, since Zeto_Anon now links ZetoLockableLib.
			zetoPoolABI := solutils.MustParseBuildABI(zetoAnonBuildJSON)

			// V1 pins the spend recipient when the lock is created, not when it is spent — that is what lets the
			// delegate's spendLock be authorised ahead of time. Prepare rather than Send so this test submits the
			// public leg itself and therefore knows the msg.sender, which becomes the lock's spender.
			createLock := rpc[paladinPrefix+"1"].ForABI(ctx, zetoFungibleABI).
				Private().
				Domain("zeto").
				Function("createLock").
				To(zetoContract).
				From(bobID).
				Inputs(&zetotypes.CreateLockParams{
					From: bobID,
					Recipients: []*zetotypes.FungibleTransferParamEntry{
						{To: fredID, Amount: with10Decimals(10)},
					},
					UnlockData: pldtypes.HexBytes{},
					Data:       pldtypes.HexBytes{},
				}).
				Prepare().
				Wait(5 * time.Second)
			Expect(createLock.Error()).To(BeNil())

			createLockPublic := rpc[paladinPrefix+"1"].ForABI(ctx, zetoPoolABI).
				Public().
				Function("createLock").
				To(zetoContract).
				From(bobID).
				Inputs(createLock.PreparedTransaction().Transaction.Data).
				Send().
				Wait(5 * time.Second)
			Expect(createLockPublic.Error()).To(BeNil())
			testLog("Zeto createLock transaction %s", createLockPublic.ID())
			logWallet("bob", paladinPrefix+"1")

			// The lock id is carried on the persisted lock-info state, which is the only place it is available
			// over plain RPC (the domain also emits it on ZetoLockCreated).
			var lockInfos []*zetoLockInfoStateResult
			Eventually(func() int {
				err := rpc[paladinPrefix+"1"].CallRPC(ctx, &lockInfos, "pstate_queryContractStates", "zeto", zetoContract,
					zetoLockInfoSchemaID, query.NewQueryBuilder().Limit(1).Query(), "all")
				Expect(err).To(BeNil())
				return len(lockInfos)
			}, "20s", "1s").Should(Equal(1))
			lockID := lockInfos[0].Data.LockID
			Expect(lockID.IsZero()).To(BeFalse())
			testLog("Zeto lock %s created (spender %s)", lockID, lockInfos[0].Data.Spender)

			// Hand the lock to sally so she — not bob — can spend it. Only the current spender may delegate.
			delegateArgs, err := zetoDelegateLockArgsABI.EncodeABIDataJSONCtx(ctx, []byte(fmt.Sprintf(
				`[{"txId":"%s"}]`, pldtypes.Bytes32UUIDFirst16(uuid.New()).HexString0xPrefix())))
			Expect(err).To(BeNil())
			delegate := rpc[paladinPrefix+"1"].ForABI(ctx, zetoPoolABI).
				Public().
				Function("delegateLock").
				To(zetoContract).
				From(bobID).
				Inputs(map[string]any{
					"lockId":       lockID.HexString0xPrefix(),
					"delegateArgs": pldtypes.HexBytes(delegateArgs).HexString0xPrefix(),
					"newSpender":   sallyEthAddr.String(),
					"data":         "0x",
				}).
				Send().
				Wait(5 * time.Second)
			Expect(delegate.Error()).To(BeNil())
			testLog("Zeto delegateLock transaction %s", delegate.ID())

			// Bob owns the locked value so he generates the proof; sally submits it as the delegate.
			spend := rpc[paladinPrefix+"1"].ForABI(ctx, zetoFungibleABI).
				Private().
				Domain("zeto").
				Function("spendLock").
				To(zetoContract).
				From(bobID).
				Inputs(&zetotypes.SpendLockParams{
					LockId: lockID,
					From:   bobID,
					Data:   pldtypes.HexBytes{},
				}).
				Prepare().
				Wait(5 * time.Second)
			Expect(spend.Error()).To(BeNil())

			spendPublic := rpc[paladinPrefix+"2"].ForABI(ctx, zetoPoolABI).
				Public().
				Function("spendLock").
				To(zetoContract).
				From(sallyID).
				Inputs(spend.PreparedTransaction().Transaction.Data).
				Send().
				Wait(5 * time.Second)
			Expect(spendPublic.Error()).To(BeNil())
			testLog("Zeto spendLock transaction %s", spendPublic.ID())
			logWallet("fred", paladinPrefix+"3")
		})
	})
})
