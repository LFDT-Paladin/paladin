import { copyFile } from "copy-file";

await copyFile(
  "../../solidity/artifacts/@openzeppelin/contracts/proxy/ERC1967/ERC1967Proxy.sol/ERC1967Proxy.json",
  "contracts/abis/ERC1967Proxy.json"
);
