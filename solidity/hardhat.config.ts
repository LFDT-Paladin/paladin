import "@nomicfoundation/hardhat-toolbox";
import "@openzeppelin/hardhat-upgrades";
import "@typechain/hardhat";
import { HardhatUserConfig } from "hardhat/config";

const config: HardhatUserConfig = {
  solidity: {
    compilers: [
      {
        version: "0.8.27",
        settings: {
          evmVersion: "shanghai",
          optimizer: {
            enabled: false,
            runs: 1000,
          },
          viaIR: true,
        },
      },
      {
        version: "0.8.30",
        settings: {
          evmVersion: "shanghai",
          optimizer: {
            enabled: false,
            runs: 1000,
          },
          viaIR: true,
        },
      }
    ]
  },
  networks: {
    hardhat: {
      // a small number of verifiers are bigger than the default 24576 bytes
      // so we need to allow unlimited contract size to avoid test errors
      allowUnlimitedContractSize: true,
    },
  },
};

export default config;
