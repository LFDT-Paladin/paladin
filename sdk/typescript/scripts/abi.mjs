import { copyFile } from 'copy-file';
import path from 'path';

await copyFile('../../solidity/artifacts/contracts/domains/pente/PentePrivacyGroup.sol/PentePrivacyGroup.json', 'src/domains/abis/PentePrivacyGroup.json');

await copyFile('../../solidity/artifacts/contracts/domains/interfaces/INoto.sol/INoto.json', 'src/domains/abis/INoto.json');

await copyFile('../../solidity/artifacts/contracts/domains/interfaces/INotoPrivate.sol/INotoPrivate.json', 'src/domains/abis/INotoPrivate.json');

await copyFile('../../solidity/artifacts/contracts/domains/interfaces/IZetoFungible_V0.sol/IZetoFungible_V0.json', 'src/domains/abis/IZetoFungible.json');

await copyFile('../../solidity/artifacts/contracts/domains/interfaces/IZetoFungible_V1.sol/IZetoFungible_V1.json', 'src/domains/abis/IZetoFungible_V1.json');

// Upstream zeto ABIs come from the release tree Gradle already extracts for the build, rather than a separate
// download pinned to its own version. downloadZetoAbis defaulted to v0.2.0, which left the SDK carrying the V0
// pool API (delegateLock(uint256[],address,bytes)) long after the deployed contracts moved on — calls built from
// it fail at the node with "Input map missing key 'utxos'".
// Keep this root aligned with zetoVersions[].zkpRoot in domains/zeto/build.gradle.
const zetoZkpRoot = '../../domains/zeto/zkp/v0.5.1/artifacts/contracts';

await copyFile(path.join(zetoZkpRoot, 'zeto_anon.sol/Zeto_Anon.json'), 'src/domains/abis/Zeto_Anon.json');
await copyFile(path.join(zetoZkpRoot, 'lib/interfaces/IZetoKyc.sol/IZetoKyc.json'), 'src/domains/abis/IZetoKyc.json');
await copyFile(path.join(zetoZkpRoot, 'erc20.sol/SampleERC20.json'), 'src/domains/abis/SampleERC20.json');
