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
import PaladinClient, {
  INotoDomainReceipt,
  NotoBalanceOfResult,
  NotoFactory,
  PenteFactory,
  TransactionType,
} from "@lfdecentralizedtrust/paladin-sdk";
import {
  checkDeploy,
  checkReceipt,
  getCachePath,
  DEFAULT_POLL_TIMEOUT,
  LONG_POLL_TIMEOUT,
  POLL_INTERVAL,
} from "paladin-example-common";
import atomJson from "./abis/Atom.json";
import atomFactoryJson from "./abis/AtomFactory.json";
import bondTrackerPublicJson from "./abis/BondTrackerPublic.json";
import { newBondSubscription } from "./helpers/bondsubscription";
import { newBondTracker } from "./helpers/bondtracker";
import * as fs from "fs";
import * as path from "path";
import * as readline from "readline";
import { ContractData } from "./tests/data-persistence";
import { nodeConnections } from "paladin-example-common";

const logger = console;

// Print what is about to happen, then wait for the user to press Enter before
// continuing. When not attached to an interactive terminal (e.g. CI runs) the
// pause is skipped so automated runs are not blocked.
async function pause(message: string): Promise<void> {
  logger.log(`\n>>> ${message}`);
  if (!process.stdin.isTTY) {
    return;
  }
  await new Promise<void>((resolve) => {
    const rl = readline.createInterface({
      input: process.stdin,
      output: process.stdout,
    });
    rl.question("    Press Enter to continue...", () => {
      rl.close();
      resolve();
    });
  });
}

async function main(): Promise<boolean> {
  // --- Initialization from Imported Config ---
  if (nodeConnections.length < 3) {
    logger.error(
      "The environment config must provide at least 3 nodes for this scenario.",
    );
    return false;
  }

  logger.log(
    "Initializing Paladin clients from the environment configuration...",
  );
  const clients = nodeConnections.map(
    (node) => new PaladinClient(node.clientOptions),
  );
  const [paladin1, paladin2, paladin3] = clients;

  const [cashIssuer, bondIssuer] = paladin1.getVerifiers(
    `cashIssuer@${nodeConnections[0].id}`,
    `bondIssuer@${nodeConnections[0].id}`,
  );

  const [bondCustodian] = paladin2.getVerifiers(
    `bondCustodian@${nodeConnections[1].id}`,
  );
  const [investor] = paladin3.getVerifiers(`investor@${nodeConnections[2].id}`);
  // Create a Noto token to represent cash
  logger.log(`Deploying Noto cash token (notary: ${cashIssuer.lookup})...`);
  const notoFactory = new NotoFactory(paladin1, "noto");
  const notoCash = await notoFactory
    .newNoto(cashIssuer, {
      name: "BOND",
      symbol: "BOND",
      notary: cashIssuer,
      notaryMode: "basic",
    })
    .waitForDeploy(DEFAULT_POLL_TIMEOUT);
  if (!checkDeploy(notoCash)) return false;

  // Issue some cash
  logger.log(
    `Issuing cash to ${investor.lookup} from ${cashIssuer.lookup}...`,
  );
  let receipt = await notoCash
    .mint(cashIssuer, {
      to: investor,
      amount: 100000,
      data: "0x",
    })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT);
  if (!checkReceipt(receipt)) return false;

  let balanceInvestor = await notoCash.balanceOf(cashIssuer, {
    account: investor.lookup,
  });
  logger.log(
    `(NotoCash) ${investor.lookup} balance: ${balanceInvestor.totalBalance} units of cash, ${balanceInvestor.totalStates} states, overflow: ${balanceInvestor.overflow}`,
  );

  await pause(
    "About to create a Pente privacy group for the bond issuer and custodian.",
  );

  // Create a Pente privacy group between the bond issuer and bond custodian
  logger.log(
    `Creating issuer+custodian privacy group (members: ${bondIssuer.lookup}, ${bondCustodian.lookup})...`,
  );
  const penteFactory = new PenteFactory(paladin1, "pente");
  const issuerCustodianGroup = await penteFactory
    .newPrivacyGroup({
      members: [bondIssuer, bondCustodian],
      evmVersion: "shanghai",
      externalCallsEnabled: true,
    })
    .waitForDeploy(DEFAULT_POLL_TIMEOUT);
  if (!checkDeploy(issuerCustodianGroup)) return false;

  // Deploy the public bond tracker on the base ledger (controlled by the privacy group)
  logger.log(
    `Creating public bond tracker (deployer: ${bondIssuer.lookup})...`,
  );
  const issueDate = Math.floor(Date.now() / 1000);
  const maturityDate = issueDate + 60 * 60 * 24;
  let txID = await paladin1.ptx.sendTransaction({
    type: TransactionType.PUBLIC,
    abi: bondTrackerPublicJson.abi,
    bytecode: bondTrackerPublicJson.bytecode,
    function: "",
    from: bondIssuer.lookup,
    data: {
      owner: issuerCustodianGroup.address,
      issueDate_: issueDate,
      maturityDate_: maturityDate,
      currencyToken_: notoCash.address,
      faceValue_: 1,
    },
  });
  receipt = await paladin1.pollForReceipt(txID, DEFAULT_POLL_TIMEOUT);
  if (receipt?.contractAddress === undefined) {
    logger.error("Failed!");
    return false;
  }
  logger.log(`Success! address: ${receipt.contractAddress}`);
  const bondTrackerPublicAddress = receipt.contractAddress;

  // Deploy private bond tracker to the issuer/custodian privacy group
  logger.log(
    `Creating private bond tracker in the issuer+custodian group (deployer: ${bondIssuer.lookup}, custodian: ${bondCustodian.lookup})...`,
  );
  const bondTracker = await newBondTracker(issuerCustodianGroup, bondIssuer, {
    name: "BOND",
    symbol: "BOND",
    custodian: await bondCustodian.address(),
    publicTracker: bondTrackerPublicAddress,
  });
  if (!checkDeploy(bondTracker)) return false;

  await pause(
    "About to create a Noto token (representing the bond) in hooks mode, backed by the private bond tracker.",
  );

  // Deploy Noto token to represent bond
  logger.log(
    `Deploying Noto bond token (deployer: ${bondIssuer.lookup}, notary: ${bondCustodian.lookup})...`,
  );
  const notoBond = await notoFactory
    .newNoto(bondIssuer, {
      name: "BOND",
      symbol: "BOND",
      notary: bondCustodian,
      notaryMode: "hooks",
      options: {
        hooks: {
          privateGroup: issuerCustodianGroup,
          publicAddress: issuerCustodianGroup.address,
          privateAddress: bondTracker.address,
        },
      },
    })
    .waitForDeploy(DEFAULT_POLL_TIMEOUT);
  if (!checkDeploy(notoBond)) return false;

  await pause("About to create an atom factory on the base ledger.");

  // Deploy the atom factory on the base ledger
  logger.log(`Creating atom factory (deployer: ${bondIssuer.lookup})...`);
  txID = await paladin1.ptx.sendTransaction({
    type: TransactionType.PUBLIC,
    abi: atomFactoryJson.abi,
    bytecode: atomFactoryJson.bytecode,
    function: "",
    from: bondIssuer.lookup,
    data: {},
  });
  receipt = await paladin1.pollForReceipt(txID, DEFAULT_POLL_TIMEOUT);
  if (receipt?.contractAddress === undefined) {
    logger.error("Failed!");
    return false;
  }
  logger.log(`Success! address: ${receipt.contractAddress}`);
  const atomFactoryAddress = receipt.contractAddress;

  await pause("About to issue the bond to the custodian.");

  // Issue the bond to the custodian
  logger.log(
    `Issuing bond to ${bondCustodian.lookup} from ${bondIssuer.lookup}...`,
  );
  receipt = await notoBond
    .mint(bondIssuer, {
      to: bondCustodian,
      amount: 1000,
      data: "0x",
    })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT);
  if (!checkReceipt(receipt)) return false;
  let balanceCustodian = await notoBond.balanceOf(bondIssuer, {
    account: bondCustodian.lookup,
  });
  logger.log(
    `(NotoBond) ${bondCustodian.lookup} balance: ${balanceCustodian.totalBalance} units of bonds, ${balanceCustodian.totalStates} states, overflow: ${balanceCustodian.overflow}`,
  );

  await pause(
    "About to begin distributing the bond, starting by registering the allowed investors.",
  );

  // Begin bond distribution to investors
  logger.log(
    `Beginning distribution (custodian: ${bondCustodian.lookup})...`,
  );
  receipt = await bondTracker
    .using(paladin2)
    .beginDistribution(bondCustodian, {
      discountPrice: 1,
      minimumDenomination: 1,
    })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT);
  if (!checkReceipt(receipt)) return false;

  // Add allowed investors
  const investorList = await bondTracker.investorList(bondIssuer);
  receipt = await investorList
    .using(paladin2)
    .addInvestor(bondCustodian, { addr: await investor.address() })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT);
  if (!checkReceipt(receipt)) return false;

  await pause(
    "About to create a Pente privacy group between the custodian and the investor.",
  );

  // Create a Pente privacy group between the bond investor and bond custodian
  logger.log(
    `Creating investor+custodian privacy group (members: ${investor.lookup}, ${bondCustodian.lookup})...`,
  );
  const investorCustodianGroup = await penteFactory
    .using(paladin3)
    .newPrivacyGroup({
      members: [investor, bondCustodian],
      evmVersion: "shanghai",
      externalCallsEnabled: true,
    })
    .waitForDeploy(DEFAULT_POLL_TIMEOUT);
  if (investorCustodianGroup === undefined) {
    logger.error("Failed!");
    return false;
  }
  logger.log(`Success! address: ${investorCustodianGroup.address}`);

  // Deploy bond subscription to the investor/custodian privacy group
  logger.log(
    `Creating private bond subscription in the investor+custodian group (deployer: ${investor.lookup}, custodian: ${bondCustodian.lookup})...`,
  );
  const bondSubscription = await newBondSubscription(
    investorCustodianGroup,
    investor,
    {
      bondAddress_: notoBond.address,
      units_: 100,
      custodian_: await bondCustodian.address(),
      atomFactory_: atomFactoryAddress,
    },
  );
  if (!checkDeploy(bondSubscription)) return false;

  await pause(
    "About to lock the investor's cash ready for DvP, and pre-create the unlock operation.",
  );

  // Prepare the payment transfer (investor -> custodian)
  logger.log(`Locking cash transfer from ${investor.lookup}...`);
  receipt = await notoCash
    .using(paladin3)
    .lock(investor, {
      amount: 100,
      data: "0x",
    })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT);
  if (!checkReceipt(receipt)) return false;
  receipt = await paladin3.ptx.getTransactionReceiptFull(receipt.id);
  let domainReceipt = receipt?.domainReceipt as INotoDomainReceipt | undefined;
  const cashLockId = domainReceipt?.lockInfo?.lockId;
  if (cashLockId === undefined) {
    logger.error("No lock ID found in domain receipt");
    return false;
  }
  balanceInvestor = await notoCash
    .using(paladin3)
    .balanceOf(investor, { account: investor.lookup });
  logger.log(
    `(NotoCash) ${investor.lookup} balance: ${balanceInvestor.totalBalance} units of cash, ${balanceInvestor.totalStates} states, overflow: ${balanceInvestor.overflow}`,
  );

  // Prepare unlock operation
  logger.log(
    `Preparing cash unlock from ${investor.lookup} to ${bondCustodian.lookup}...`,
  );
  receipt = await notoCash
    .using(paladin3)
    .prepareUnlock(investor, {
      lockId: cashLockId,
      from: investor,
      recipients: [{ to: bondCustodian, amount: 100 }],
      unlockData: "0x",
      data: "0x",
    })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT, true);
  if (!checkReceipt(receipt)) return false;
  domainReceipt = receipt?.domainReceipt as INotoDomainReceipt | undefined;
  const cashUnlockCall = domainReceipt?.lockInfo?.unlockCall;
  if (cashUnlockCall === undefined) {
    logger.error("No unlock data found in domain receipt");
    return false;
  }

  await pause(
    "About to lock the custodian's bond ready for DvP, and pre-create the unlock operation.",
  );

  // Prepare the bond transfer (custodian -> investor)
  logger.log(`Locking bond asset from ${bondCustodian.lookup}...`);
  receipt = await notoBond
    .using(paladin2)
    .lock(bondCustodian, {
      amount: 100,
      data: "0x",
    })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT, true);
  if (!checkReceipt(receipt)) return false;
  domainReceipt = receipt?.domainReceipt as INotoDomainReceipt | undefined;
  const bondLockId = domainReceipt?.lockInfo?.lockId;
  if (bondLockId === undefined) {
    logger.error("No lock ID found in domain receipt");
    return false;
  }
  balanceCustodian = await notoBond
    .using(paladin2)
    .balanceOf(bondCustodian, { account: bondCustodian.lookup });
  logger.log(
    `(NotoBond) ${bondCustodian.lookup} balance: ${balanceCustodian.totalBalance} units of bonds, ${balanceCustodian.totalStates} states, overflow: ${balanceCustodian.overflow}`,
  );

  // Prepare unlock operation
  logger.log(
    `Preparing bond unlock from ${bondCustodian.lookup} to ${investor.lookup}...`,
  );
  receipt = await notoBond
    .using(paladin2)
    .prepareUnlock(bondCustodian, {
      lockId: bondLockId,
      from: bondCustodian,
      recipients: [{ to: investor, amount: 100 }],
      unlockData: "0x",
      data: "0x",
    })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT, true);
  if (!checkReceipt(receipt)) return false;
  domainReceipt = receipt?.domainReceipt as INotoDomainReceipt | undefined;
  const assetUnlockCall = domainReceipt?.lockInfo?.unlockCall;
  if (assetUnlockCall === undefined) {
    logger.error("No unlock data found in domain receipt");
    return false;
  }

  await pause(
    "About to pass the prepared unlocks to the bond subscription contract.",
  );

  // Pass the prepared payment transfer to the subscription contract
  logger.log(
    `Adding payment information to subscription request (from ${investor.lookup})...`,
  );
  receipt = await bondSubscription
    .using(paladin3)
    .preparePayment(investor, {
      to: notoCash.address,
      encodedCall: cashUnlockCall,
    })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT);
  if (!checkReceipt(receipt)) return false;

  // Pass the prepared bond transfer to the subscription contract
  logger.log(
    `Adding bond information to subscription request (from ${bondCustodian.lookup})...`,
  );
  receipt = await bondSubscription
    .using(paladin2)
    .prepareBond(bondCustodian, {
      to: notoBond.address,
      encodedCall: assetUnlockCall,
    })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT);
  if (!checkReceipt(receipt)) return false;

  await pause(
    "About to prepare the full DvP bond distribution by creating an atomic swap with the unlocks in.",
  );

  // Prepare bond distribution (initializes atomic swap of payment and bond units)
  logger.log(
    `Generating atom for bond distribution (custodian: ${bondCustodian.lookup})...`,
  );
  receipt = await bondSubscription
    .using(paladin2)
    .distribute(bondCustodian)
    .waitForReceipt(DEFAULT_POLL_TIMEOUT);
  if (!checkReceipt(receipt)) return false;

  // Extract the address of the created Atom
  const events = await paladin2.bidx.decodeTransactionEvents(
    receipt.transactionHash,
    atomFactoryJson.abi,
    "",
  );
  const atomDeployedEvent = events.find(
    (e) => e.soliditySignature === "event AtomDeployed(address addr)",
  );
  if (atomDeployedEvent === undefined) {
    logger.error("Did not find AtomDeployed event");
    return false;
  }
  const atomAddress = atomDeployedEvent.data.addr;
  logger.log("Success!");

  await pause("About to delegate the cash lock for DvP to the atom.");

  // Approve the payment transfer
  logger.log(
    `Approving payment transfer (delegating ${investor.lookup}'s cash lock to the atom)...`,
  );
  receipt = await notoCash
    .using(paladin3)
    .delegateLock(investor, {
      lockId: cashLockId,
      delegate: atomAddress,
      data: "0x",
    })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT);
  if (!checkReceipt(receipt)) return false;

  await pause("About to delegate the bond lock for DvP to the atom.");

  // Approve the bond transfer
  logger.log(
    `Approving bond transfer (delegating ${bondCustodian.lookup}'s bond lock to the atom)...`,
  );
  receipt = await notoBond
    .using(paladin2)
    .delegateLock(bondCustodian, {
      lockId: bondLockId,
      delegate: atomAddress,
      data: "0x",
    })
    .waitForReceipt(DEFAULT_POLL_TIMEOUT);
  if (!checkReceipt(receipt)) return false;

  await pause(
    "About to execute the atom, atomically settling the cash and bond legs of the DvP.",
  );

  // Execute the atomic transfer
  logger.log(
    `Distributing bond (executing atom, submitted by ${bondCustodian.lookup})...`,
  );
  txID = await paladin2.ptx.sendTransaction({
    type: TransactionType.PUBLIC,
    abi: atomJson.abi,
    function: "execute",
    from: bondCustodian.lookup,
    to: atomAddress,
    data: {},
  });
  receipt = await paladin2.pollForReceipt(txID, DEFAULT_POLL_TIMEOUT);
  if (!checkReceipt(receipt)) return false;

  await pause(
    "DvP settled. About to read the final balances back from Paladin and show how they were retrieved.",
  );

  // it can take some time for the balances to update, so loop until all balances are >0
  let finalCashBalanceInvestor: NotoBalanceOfResult | undefined;
  let finalBondBalanceInvestor: NotoBalanceOfResult | undefined;
  let finalCashBalanceCustodian: NotoBalanceOfResult | undefined;
  let finalBondBalanceCustodian: NotoBalanceOfResult | undefined;
  const startTime = Date.now();
  while (true) {
    // Get final balances after the bond distribution
    finalCashBalanceInvestor = await notoCash
      .using(paladin3)
      .balanceOf(investor, { account: investor.lookup });

    finalBondBalanceInvestor = await notoBond
      .using(paladin3)
      .balanceOf(investor, { account: investor.lookup });

    finalCashBalanceCustodian = await notoCash
      .using(paladin2)
      .balanceOf(bondCustodian, { account: bondCustodian.lookup });

    finalBondBalanceCustodian = await notoBond
      .using(paladin2)
      .balanceOf(bondCustodian, { account: bondCustodian.lookup });

    if (
      finalCashBalanceInvestor?.totalBalance !== "0" &&
      finalBondBalanceInvestor?.totalBalance !== "0" &&
      finalCashBalanceCustodian?.totalBalance !== "0" &&
      finalBondBalanceCustodian?.totalBalance !== "0"
    ) {
      break;
    }

    if (Date.now() - startTime > LONG_POLL_TIMEOUT) {
      logger.error(
        `Failed to get final balances after ${LONG_POLL_TIMEOUT / 1000} seconds`,
      );
      return false;
    }

    await new Promise((resolve) => setTimeout(resolve, POLL_INTERVAL));
  }

  // a) Print out what the balances are, now the DvP has settled.
  logger.log("\n=== Final balances after DvP settlement ===");
  logger.log(
    `Investor  (${investor.lookup}) - cash: ${finalCashBalanceInvestor.totalBalance} (${finalCashBalanceInvestor.totalStates} states), ` +
      `bond: ${finalBondBalanceInvestor.totalBalance} (${finalBondBalanceInvestor.totalStates} states)`,
  );
  logger.log(
    `Custodian (${bondCustodian.lookup}) - cash: ${finalCashBalanceCustodian.totalBalance} (${finalCashBalanceCustodian.totalStates} states), ` +
      `bond: ${finalBondBalanceCustodian.totalBalance} (${finalBondBalanceCustodian.totalStates} states)`,
  );

  // b) Explain how those balances were retrieved from Paladin.
  logger.log("\n=== How these balances were retrieved from Paladin ===");
  logger.log(
    "Each balance came from the Noto domain's `balanceOf` call: the SDK method " +
      "`noto.balanceOf(from, { account })` issues a JSON-RPC `ptx_call` (TransactionType PRIVATE, " +
      "function `balanceOf`) against the Noto contract address.",
  );
  logger.log(
    "Paladin answers it by querying the account's unspent NotoCoin UTXO states (the coin schema, " +
      "filtered by `owner`) and summing their `amount` into `totalBalance` / `totalStates`.",
  );
  logger.log(
    "This is identical for the basic-mode cash token and the hooks-mode bond token: the " +
      "authoritative balance always lives in the NotoCoin states, so `balanceOf` works the same way " +
      "regardless of notary mode (the Pente bond tracker keeps its own mirror for policy, but is not " +
      "queried here).",
  );

  // Save contract data to file for later use
  const contractData: ContractData = {
    notoCashAddress: notoCash.address,
    notoBondAddress: notoBond.address,
    issuerCustodianGroupId: issuerCustodianGroup.group.id,
    issuerCustodianGroupAddress: issuerCustodianGroup.address,
    investorCustodianGroupId: investorCustodianGroup.group.id,
    investorCustodianGroupAddress: investorCustodianGroup.address,
    bondTrackerAddress: bondTracker.address,
    bondTrackerPublicAddress: bondTrackerPublicAddress,
    bondSubscriptionAddress: bondSubscription.address,
    atomFactoryAddress: atomFactoryAddress,
    atomAddress: atomAddress,
    bondDetails: {
      issueDate: issueDate,
      maturityDate: maturityDate,
      faceValue: 1,
      discountPrice: 1,
      minimumDenomination: 1,
      bondUnits: 100,
      cashAmount: 100,
    },
    lockDetails: {
      cashLockId: cashLockId,
      bondLockId: bondLockId,
      cashUnlockCall: cashUnlockCall,
      assetUnlockCall: assetUnlockCall,
    },
    finalBalances: {
      cash: {
        investor: {
          totalBalance: finalCashBalanceInvestor.totalBalance,
          totalStates: finalCashBalanceInvestor.totalStates,
          overflow: finalCashBalanceInvestor.overflow,
        },
        custodian: {
          totalBalance: finalCashBalanceCustodian.totalBalance,
          totalStates: finalCashBalanceCustodian.totalStates,
          overflow: finalCashBalanceCustodian.overflow,
        },
      },
      bond: {
        investor: {
          totalBalance: finalBondBalanceInvestor.totalBalance,
          totalStates: finalBondBalanceInvestor.totalStates,
          overflow: finalBondBalanceInvestor.overflow,
        },
        custodian: {
          totalBalance: finalBondBalanceCustodian.totalBalance,
          totalStates: finalBondBalanceCustodian.totalStates,
          overflow: finalBondBalanceCustodian.overflow,
        },
      },
    },
    participants: {
      cashIssuer: cashIssuer.lookup,
      bondIssuer: bondIssuer.lookup,
      bondCustodian: bondCustodian.lookup,
      investor: investor.lookup,
    },
    timestamp: new Date().toISOString(),
  };

  // Use command-line argument for data directory if provided, otherwise use default
  const dataDir = getCachePath();
  if (!fs.existsSync(dataDir)) {
    fs.mkdirSync(dataDir, { recursive: true });
  }

  const timestamp = new Date().toISOString().replace(/[:.]/g, "-");
  const dataFile = path.join(dataDir, `contract-data-${timestamp}.json`);
  fs.writeFileSync(dataFile, JSON.stringify(contractData, null, 2));
  logger.log(`Contract data saved to ${dataFile}`);

  return true;
}

if (require.main === module) {
  main()
    .then((success: boolean) => {
      process.exit(success ? 0 : 1);
    })
    .catch((err) => {
      console.error("Exiting with uncaught error");
      console.error(err);
      process.exit(1);
    });
}
