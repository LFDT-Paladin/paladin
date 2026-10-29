// Copyright contributors to Paladin, an LFDT project
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import {
  PaladinVerifier,
  PentePrivacyGroup,
} from "@lfdecentralizedtrust/paladin-sdk";
import { ethers } from "ethers";
import { DEFAULT_POLL_TIMEOUT } from "./config";
import erc1967Proxy from "../contracts/abis/ERC1967Proxy.json";

export interface ContractBuild {
  abi: ReadonlyArray<ethers.JsonFragment>;
  bytecode: string;
}

// The proxy calls initialize as it is created, so nobody else can initialize it first
const encodeInitialize = (build: ContractBuild, params: Record<string, any>) => {
  const contract = new ethers.Interface(build.abi);
  const initialize = contract.getFunction("initialize")!;
  return contract.encodeFunctionData(
    initialize,
    initialize.inputs.map((input) => params[input.name])
  );
};

export const deployPrivateProxy = async (
  pente: PentePrivacyGroup,
  from: PaladinVerifier,
  build: ContractBuild,
  params: Record<string, any>
) => {
  const implementation = await pente
    .deploy({ abi: build.abi, bytecode: build.bytecode, from: from.lookup })
    .waitForDeploy(DEFAULT_POLL_TIMEOUT);
  if (!implementation) return undefined;
  return pente.deploy({
    abi: erc1967Proxy.abi,
    bytecode: erc1967Proxy.bytecode,
    from: from.lookup,
    inputs: {
      implementation,
      _data: encodeInitialize(build, params),
    },
  });
};
