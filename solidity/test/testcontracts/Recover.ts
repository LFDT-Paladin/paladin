import { expect } from "chai";
import { ethers } from "hardhat";

describe("Recover", function () {
  it("recovers as expected", async function () {
    const Recover = await ethers.getContractFactory("Recover");
    const recover = await Recover.deploy();

    const res = await recover.verifySignature.call(null,
      "0xacaf3289d7b601cbd114fb36c4d29c85bbfd5e133f14cb355c3fd8d99367964f",
      "0xe76d4a6f194440ca1b19695e41538b960afc9c27c69722ef93cbf0134cbc6fd317481bff0ca56883b81fd37dcf009d7b9c98c793a67826602b8d8eb83a8b94c51b",
      "0x78826125b6be403ea159876f5a32a3eac7cd0fe5"
    );
    await expect(res.valid).to.true;
    await expect(res.recoveredSigner).to.equal("0x78826125b6be403ea159876f5a32a3eac7cd0fe5");
  });
});
