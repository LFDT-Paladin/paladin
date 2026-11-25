import { loadFixture } from "@nomicfoundation/hardhat-toolbox/network-helpers";
import { expect } from "chai";
import { id, ZeroAddress } from "ethers";
import { ethers } from "hardhat";

// ZetoFactory_V1 is the upgradeable Paladin wrapper the Zeto domain plugin deploys through, and ZetoFactory_V0 the
// non-upgradeable one it replaced. These tests pin the two compatibility guarantees documented on ZetoFactory_V1:
//   1. both factories can deploy implementations compiled against zeto-contracts 0.2.2 AND 0.5.1
//   2. ZetoFactory_V1's `deploy` selectors still match ZetoFactory_V0's

// Any non-zero address works: the factory only null-checks the verifiers and forwards them to initialize().
const verifiers = (base: number) => ({
  verifier: `0x${(base + 1).toString(16).padStart(40, "0")}`,
  depositVerifier: `0x${(base + 2).toString(16).padStart(40, "0")}`,
  withdrawVerifier: `0x${(base + 3).toString(16).padStart(40, "0")}`,
  lockVerifier: `0x${(base + 4).toString(16).padStart(40, "0")}`,
  burnVerifier: `0x${(base + 5).toString(16).padStart(40, "0")}`,
  batchVerifier: `0x${(base + 6).toString(16).padStart(40, "0")}`,
  batchWithdrawVerifier: `0x${(base + 7).toString(16).padStart(40, "0")}`,
  batchLockVerifier: `0x${(base + 8).toString(16).padStart(40, "0")}`,
  batchBurnVerifier: `0x${(base + 9).toString(16).padStart(40, "0")}`,
});

async function registerImplementations(factory: any) {
  const v022Impl = await (await ethers.getContractFactory("MockZetoTokenV022")).deploy();
  const v051Impl = await (await ethers.getContractFactory("MockZetoTokenV051")).deploy();
  await factory.registerImplementation("zeto_v0_2_2", {
    implementation: await v022Impl.getAddress(),
    verifiers: verifiers(0x1000),
  });
  await factory.registerImplementation("zeto_v0_5_1", {
    implementation: await v051Impl.getAddress(),
    verifiers: verifiers(0x2000),
  });
}

async function deployZetoFactoryV1Fixture() {
  const [deployer, tokenOwner] = await ethers.getSigners();
  // implementation + ERC1967Proxy(initialize()) — the flow the operator's contract map and the integration-test
  // helpers both use
  const ZetoFactoryV1 = await ethers.getContractFactory("ZetoFactory_V1");
  const impl = await ZetoFactoryV1.deploy();
  const initCalldata = ZetoFactoryV1.interface.encodeFunctionData("initialize");
  const ERC1967Proxy = await ethers.getContractFactory("ERC1967Proxy");
  const proxy = await ERC1967Proxy.deploy(await impl.getAddress(), initCalldata);
  const factory = ZetoFactoryV1.attach(await proxy.getAddress()) as any;
  await registerImplementations(factory);
  return { factory, deployer, tokenOwner };
}

async function deployZetoFactoryV0Fixture() {
  const [deployer, tokenOwner] = await ethers.getSigners();
  const factory = (await (await ethers.getContractFactory("ZetoFactory_V0")).deploy()) as any;
  await registerImplementations(factory);
  return { factory, deployer, tokenOwner };
}

for (const [contractName, fixture] of [
  ["ZetoFactory_V1", deployZetoFactoryV1Fixture],
  ["ZetoFactory_V0", deployZetoFactoryV0Fixture],
] as const) {
  describe(contractName, function () {
    // The factory clones an implementation and calls IZetoInitializable.initialize(string,string,address,(address x9)).
    // That selector is unchanged from 0.2.2 through 0.5.1, which is what lets one factory serve both variants.
    for (const [label, tokenName, base] of [
      ["zeto-contracts 0.2.2", "zeto_v0_2_2", 0x1000],
      ["zeto-contracts 0.5.1", "zeto_v0_5_1", 0x2000],
    ] as const) {
      it(`deploys a fungible token implementation built against ${label}`, async function () {
        const { factory, tokenOwner } = await loadFixture(fixture);
        const txId = id(tokenName);
        const config = "0xfeed";

        const tx = await factory["deploy(bytes32,string,string,string,address,bytes,bool)"](
          txId, tokenName, "Token", "TOK", tokenOwner.address, config, false,
        );
        const receipt = await tx.wait();

        const registered = receipt!.logs
          .map((log: any) => { try { return factory.interface.parseLog(log); } catch { return null; } })
          .find((parsed: any) => parsed?.name === "PaladinRegisterSmartContract_V0");
        expect(registered, "PaladinRegisterSmartContract_V0 not emitted").to.not.be.undefined;
        expect(registered!.args.txId).to.equal(txId);
        expect(registered!.args.config).to.equal(config);

        // the clone was initialized with exactly what the factory held for this implementation
        const instance = registered!.args.instance;
        expect(instance).to.not.equal(ZeroAddress);
        const token = await ethers.getContractAt("MockZetoTokenV022", instance);
        expect(await token.initialized()).to.equal(true);
        expect(await token.name()).to.equal("Token");
        expect(await token.symbol()).to.equal("TOK");
        expect(await token.initialOwner()).to.equal(tokenOwner.address);
        expect((await token.verifier()).toLowerCase()).to.equal(verifiers(base).verifier);
      });
    }

    it("deploys a non-fungible token through the same initialize entry point", async function () {
      const { factory, tokenOwner } = await loadFixture(fixture);
      const tx = await factory["deploy(bytes32,string,string,string,address,bytes,bool)"](
        id("nft"), "zeto_v0_5_1", "NFT", "NFT", tokenOwner.address, "0x", true,
      );
      const receipt = await tx.wait();
      const registered = receipt!.logs
        .map((log: any) => { try { return factory.interface.parseLog(log); } catch { return null; } })
        .find((parsed: any) => parsed?.name === "PaladinRegisterSmartContract_V0");
      expect(registered, "PaladinRegisterSmartContract_V0 not emitted").to.not.be.undefined;

      const token = await ethers.getContractAt("MockZetoTokenV051", registered!.args.instance);
      expect(await token.initialized()).to.equal(true);
      expect(await token.name()).to.equal("NFT");
    });

    it("defaults to a fungible deploy on the 6-argument overload", async function () {
      const { factory, tokenOwner } = await loadFixture(fixture);
      await expect(
        factory["deploy(bytes32,string,string,string,address,bytes)"](
          id("default"), "zeto_v0_2_2", "Token", "TOK", tokenOwner.address, "0x",
        ),
      ).to.emit(factory, "PaladinRegisterSmartContract_V0");
    });
  });
}

describe("ZetoFactory_V1 upgradeability", function () {
  it("initializes through the ERC1967 proxy with the deployer as owner", async function () {
    const { factory, deployer } = await loadFixture(deployZetoFactoryV1Fixture);
    expect(await factory.owner()).to.equal(deployer.address);
    // _disableInitializers() in the inherited constructor plus a single `initializer` modifier: re-initializing the
    // live proxy must revert rather than silently re-running
    await expect(factory.initialize()).to.be.revertedWithCustomError(factory, "InvalidInitialization");
  });

  it("keeps the ZetoFactory_V0 deploy and registerImplementation selectors", async function () {
    // The domain plugin encodes calls with this ABI against ZetoFactory_V0 addresses already deployed. Those contracts
    // expose no initialize()/upgradeToAndCall(), but do expose these entry points.
    const { interface: v1 } = await ethers.getContractFactory("ZetoFactory_V1");
    const { interface: v0 } = await ethers.getContractFactory("ZetoFactory_V0");
    for (const signature of [
      "deploy(bytes32,string,string,string,address,bytes,bool)",
      "deploy(bytes32,string,string,string,address,bytes)",
      "registerImplementation(string,(address,(address,address,address,address,address,address,address,address,address)))",
    ]) {
      expect(v1.getFunction(signature)!.selector, signature).to.equal(v0.getFunction(signature)!.selector);
    }
  });
});
