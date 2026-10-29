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

import { expect } from "chai";
import { ethers } from "hardhat";
import { BondTracker, InvestorList } from "../../typechain-types";
import { deployUpgradeableContract } from "../helpers/deploy-upgradeable";

describe("BondTracker", function () {
  async function deployBondTracker() {
    const [issuer, custodian, publicTracker] = await ethers.getSigners();
    const bondTracker = (await deployUpgradeableContract(
      "BondTracker",
      "initialize",
      ["BOND", "BOND", custodian.address, publicTracker.address],
      issuer,
    )) as BondTracker;
    const investorList = (await ethers.getContractFactory("InvestorList")).attach(
      await bondTracker.investorList(),
    ) as InvestorList;
    return { bondTracker, investorList, issuer, custodian };
  }

  describe("upgrades", function () {
    it("lets the issuer upgrade the tracker and keeps its investor list", async function () {
      const { bondTracker, investorList, issuer } = await deployBondTracker();
      const newImplementation = await (await ethers.getContractFactory("BondTracker")).deploy();

      await bondTracker.connect(issuer).upgradeToAndCall(newImplementation, "0x");
      expect(await bondTracker.investorList()).to.equal(await investorList.getAddress());
    });

    it("rejects tracker upgrades by anyone but the issuer", async function () {
      const { bondTracker, custodian } = await deployBondTracker();
      const newImplementation = await (await ethers.getContractFactory("BondTracker")).deploy();

      await expect(bondTracker.connect(custodian).upgradeToAndCall(newImplementation, "0x"))
        .to.be.revertedWithCustomError(bondTracker, "OwnableUnauthorizedAccount")
        .withArgs(custodian.address);
    });
  });
});
