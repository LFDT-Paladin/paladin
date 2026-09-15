import { loadFixture } from "@nomicfoundation/hardhat-toolbox/network-helpers";
import { expect } from "chai";
import { id, ZeroAddress } from "ethers";
import { ethers } from "hardhat";

// ZetoFactory_V0 is the single Paladin wrapper for every factoryVersion the Zeto domain plugin supports. These tests pin
// the two compatibility guarantees documented on the contract:
//   1. it can deploy implementations compiled against zeto-contracts 0.2.2 AND 0.5.1
//   2. its `deploy` selectors still match the legacy, non-upgradeable factory that predates the upgradeable wrapper
describe("ZetoFactory_V0", function () {
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

  async function deployZetoFactoryFixture() {
    const [deployer, tokenOwner] = await ethers.getSigners();

    // implementation + ERC1967Proxy(initialize()) — the flow the operator's contract map and the integration-test
    // helpers both use
    const ZetoFactory = await ethers.getContractFactory("ZetoFactory_V0");
    const impl = await ZetoFactory.deploy();
    const initCalldata = ZetoFactory.interface.encodeFunctionData("initialize");
    const ERC1967Proxy = await ethers.getContractFactory("ERC1967Proxy");
    const proxy = await ERC1967Proxy.deploy(await impl.getAddress(), initCalldata);
    const factory = ZetoFactory.attach(await proxy.getAddress()) as any;

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

    return { factory, deployer, tokenOwner };
  }

  it("initializes through the ERC1967 proxy with the deployer as owner", async function () {
    const { factory, deployer } = await loadFixture(deployZetoFactoryFixture);
    expect(await factory.owner()).to.equal(deployer.address);
    // _disableInitializers() in the inherited constructor plus a single `initializer` modifier: re-initializing the
    // live proxy must revert rather than silently re-running
    await expect(factory.initialize()).to.be.revertedWithCustomError(factory, "InvalidInitialization");
  });

  // The factory clones an implementation and calls IZetoInitializable.initialize(string,string,address,(address x9)).
  // That selector is unchanged from 0.2.2 through 0.5.1, which is what lets one factory serve both generations.
  for (const [label, tokenName, base] of [
    ["zeto-contracts 0.2.2", "zeto_v0_2_2", 0x1000],
    ["zeto-contracts 0.5.1", "zeto_v0_5_1", 0x2000],
  ] as const) {
    it(`deploys a fungible token implementation built against ${label}`, async function () {
      const { factory, tokenOwner } = await loadFixture(deployZetoFactoryFixture);
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
    const { factory, tokenOwner } = await loadFixture(deployZetoFactoryFixture);
    const txId = id("nft");

    const tx = await factory["deploy(bytes32,string,string,string,address,bytes,bool)"](
      txId, "zeto_v0_5_1", "NFT", "NFT", tokenOwner.address, "0x", true,
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
    const { factory, tokenOwner } = await loadFixture(deployZetoFactoryFixture);
    await expect(
      factory["deploy(bytes32,string,string,string,address,bytes)"](
        id("default"), "zeto_v0_2_2", "Token", "TOK", tokenOwner.address, "0x",
      ),
    ).to.emit(factory, "PaladinRegisterSmartContract_V0");
  });

  it("keeps the deploy selectors of the legacy non-upgradeable factory", async function () {
    // The domain plugin encodes calls with this ABI against factory addresses deployed before the wrapper was made
    // upgradeable. Those contracts expose no initialize()/upgradeToAndCall(), but do expose these two entry points.
    const { interface: iface } = await ethers.getContractFactory("ZetoFactory_V0");
    expect(iface.getFunction("deploy(bytes32,string,string,string,address,bytes,bool)")!.selector).to.equal("0x653bf99c");
    expect(iface.getFunction("deploy(bytes32,string,string,string,address,bytes)")!.selector).to.equal("0x05c98c83");
    expect(
      iface.getFunction(
        "registerImplementation(string,(address,(address,address,address,address,address,address,address,address,address)))",
      )!.selector,
    ).to.equal("0x3924a044");
    expect(iface.getFunction("initialize()")!.selector).to.equal("0x8129fc1c");
  });
});
