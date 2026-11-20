/*
 * Copyright contributors to Paladin, an LFDT project
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
import PaladinClient, {
  algorithmZetoSnarkBJJ,
  IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
  PaladinVerifier,
  ZetoNonFungibleInstance,
} from "@lfdecentralizedtrust/paladin-sdk";
import { checkReceipt, nodeConnections, DEFAULT_POLL_TIMEOUT, POLL_INTERVAL } from "paladin-example-common";

const logger = console;

export interface Holder {
  paladin: PaladinClient;
  verifier: PaladinVerifier;
}

// One holder on each node: alice on node1, bob on node2 and carol on node3
export function newHolders(clients: PaladinClient[]): Holder[] {
  return ["alice", "bob", "carol"].map((name, i) => ({
    paladin: clients[i],
    verifier: clients[i].getVerifiers(`${name}@${nodeConnections[i].id}`)[0],
  }));
}

export interface ZetoNFT {
  tokenID: string;
  uri: string;
}

export function sameTokenID(a: string, b: string) {
  return BigInt(a) === BigInt(b);
}

async function nftSchemaID(paladin: PaladinClient): Promise<string> {
  const schemas = await paladin.pstate.listSchemas("zeto");
  const schema = schemas.find((s) => s.signature.startsWith("type=ZetoNFToken("));
  if (schema === undefined) {
    throw new Error("Zeto NFT state schema not found");
  }
  return schema.id;
}

// Only the owner's node holds the private state of an NFT, so this must be
// called with the client for the owner's node
export async function ownedNFTs(
  paladin: PaladinClient,
  zetoAddress: string,
  owner: PaladinVerifier
): Promise<ZetoNFT[]> {
  const ownerKey = await paladin.ptx.resolveVerifier(
    owner.lookup,
    algorithmZetoSnarkBJJ("zeto"),
    IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X
  );
  const states = await paladin.pstate.queryContractStates(
    "zeto",
    zetoAddress,
    await nftSchemaID(paladin),
    { limit: 100, eq: [{ field: "owner", value: ownerKey }] },
    "confirmed"
  );
  return states.map((s) => s.data as ZetoNFT);
}

// A transfer receipt on the sender's node does not mean the recipient's node
// has received the new state yet
export async function waitForOwnedNFTs(
  paladin: PaladinClient,
  zetoAddress: string,
  owner: PaladinVerifier,
  count: number,
  waitMs: number
): Promise<ZetoNFT[] | undefined> {
  const deadline = Date.now() + waitMs;
  while (true) {
    const nfts = await ownedNFTs(paladin, zetoAddress, owner);
    if (nfts.length === count) {
      return nfts;
    }
    if (Date.now() >= deadline) {
      return undefined;
    }
    await new Promise((resolve) => setTimeout(resolve, POLL_INTERVAL));
  }
}

// Each holder in this example holds at most one NFT of the token
export async function transferNFT(
  zeto: ZetoNonFungibleInstance,
  from: Holder,
  to: Holder,
  tokenID: string
): Promise<boolean> {
  logger.log(`Transferring NFT ${tokenID} from ${from.verifier} to ${to.verifier}...`);
  const receipt = await zeto
    .using(from.paladin)
    .transfer(from.verifier, { transfers: [{ to: to.verifier, tokenID }] })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT);
  if (!checkReceipt(receipt)) return false;

  const received = await waitForOwnedNFTs(to.paladin, zeto.address, to.verifier, 1, DEFAULT_POLL_TIMEOUT);
  if (received === undefined || !sameTokenID(received[0].tokenID, tokenID)) {
    logger.error(`${to.verifier} did not receive NFT ${tokenID} on its own node`);
    return false;
  }
  const remaining = await ownedNFTs(from.paladin, zeto.address, from.verifier);
  if (remaining.length !== 0) {
    logger.error(`${from.verifier} still holds ${remaining.length} NFT(s) after the transfer`);
    return false;
  }
  logger.log(`${to.verifier} now holds NFT ${tokenID}`);
  return true;
}
