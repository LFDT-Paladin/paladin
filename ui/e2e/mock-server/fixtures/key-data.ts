import { formatHex } from './format-utils.js';

export const ROOT_KEY_COUNT = 5;
export const ORG1_KEY_COUNT = 5;
export const ORG2_KEY_COUNT = 5;
export const SUBORG1_KEY_COUNT = 5;

export const ETH_ADDRESS_TYPE = 'eth_address';
export const ETH_ADDRESS_ALGORITHM = 'ecdsa:secp256k1';
export const OTHER_VERIFIER_TYPE = 'eth_address_bytes';
export const OTHER_VERIFIER_ALGORITHM = 'ecdsa:secp256k1';

export const formatKeyEthAddress = (n: number): string => formatHex(n, 40, 'a');
export const formatKeyOtherVerifier = (n: number): string => formatHex(n, 64, 'b');
export const formatKeyHandle = (n: number): string => `m/44'/60'/0'/0/${n}`;

export interface MockVerifier {
  verifier: string;
  type: string;
  algorithm: string;
}

export interface MockKeyEntry {
  isKey: boolean;
  hasChildren: boolean;
  parent: string;
  path: string;
  name: string;
  index: number;
  wallet: string;
  keyHandle: string;
  verifiers: MockVerifier[] | null;
}

const WALLET = 'hdwallet1';

const buildFolder = (
  parent: string,
  name: string,
  index: number,
  isKey = false,
  sequence?: number
): MockKeyEntry => {
  const path = parent === '' ? name : `${parent}.${name}`;
  if (!isKey) {
    return {
      isKey: false,
      hasChildren: true,
      parent,
      path,
      name,
      index,
      wallet: '',
      keyHandle: '',
      verifiers: null,
    };
  }

  // Folder that is also a key (e.g. org2)
  return {
    isKey: true,
    hasChildren: true,
    parent,
    path,
    name,
    index,
    wallet: WALLET,
    keyHandle: formatKeyHandle(sequence ?? index),
    verifiers: [
      {
        verifier: formatKeyEthAddress(sequence ?? index),
        type: ETH_ADDRESS_TYPE,
        algorithm: ETH_ADDRESS_ALGORITHM,
      },
    ],
  };
};

const buildKey = (
  parent: string,
  name: string,
  index: number,
  sequence: number,
  includeOtherVerifier = false
): MockKeyEntry => {
  const path = parent === '' ? name : `${parent}.${name}`;
  const verifiers: MockVerifier[] = [
    {
      verifier: formatKeyEthAddress(sequence),
      type: ETH_ADDRESS_TYPE,
      algorithm: ETH_ADDRESS_ALGORITHM,
    },
  ];
  if (includeOtherVerifier) {
    verifiers.push({
      verifier: formatKeyOtherVerifier(sequence),
      type: OTHER_VERIFIER_TYPE,
      algorithm: OTHER_VERIFIER_ALGORITHM,
    });
  }

  return {
    isKey: true,
    hasChildren: false,
    parent,
    path,
    name,
    index,
    wallet: WALLET,
    keyHandle: formatKeyHandle(sequence),
    verifiers,
  };
};

/**
 * Builds key manager entries for the Keys page (list + explorer views).
 *
 * Tree layout:
 *   root/
 *     org1/                            (folder)
 *       org1key1 .. org1key5           (keys)
 *       suborg1/                       (folder)
 *         suborg1key1 .. suborg1key5   (keys)
 *     org2/                            (folder and key)
 *       org2key1 .. org2key5           (keys)
 *     rootkey1 .. rootkey5             (keys at root)
 *
 * Conventions:
 * - eth_address verifiers: 0xa...n
 * - other verifiers:       0xb...n (on every 3rd leaf key)
 * - keyHandle:             m/44'/60'/0'/0/n
 * - wallet:                hdwallet1 (keys only)
 */
export const buildKeys = (): MockKeyEntry[] => {
  const keys: MockKeyEntry[] = [];
  let sequence = 1;
  let rootIndex = 0;

  // Root folders
  keys.push(buildFolder('', 'org1', rootIndex++));
  keys.push(buildFolder('', 'org2', rootIndex++, true, sequence++));

  // Root-level keys
  for (let i = 0; i < ROOT_KEY_COUNT; i++) {
    const n = i + 1;
    keys.push(
      buildKey('', `rootkey${n}`, rootIndex++, sequence, sequence % 3 === 0)
    );
    sequence++;
  }

  // org1 children
  let org1Index = 0;
  for (let i = 0; i < ORG1_KEY_COUNT; i++) {
    const n = i + 1;
    keys.push(
      buildKey('org1', `org1key${n}`, org1Index++, sequence, sequence % 3 === 0)
    );
    sequence++;
  }
  keys.push(buildFolder('org1', 'suborg1', org1Index++));

  // suborg1 children
  for (let i = 0; i < SUBORG1_KEY_COUNT; i++) {
    const n = i + 1;
    keys.push(
      buildKey('org1.suborg1', `suborg1key${n}`, i, sequence, sequence % 3 === 0)
    );
    sequence++;
  }

  // org2 children
  for (let i = 0; i < ORG2_KEY_COUNT; i++) {
    const n = i + 1;
    keys.push(
      buildKey('org2', `org2key${n}`, i, sequence, sequence % 3 === 0)
    );
    sequence++;
  }

  return keys;
};

export const KEY_COUNT =
  2 + // org1, org2
  ROOT_KEY_COUNT +
  ORG1_KEY_COUNT +
  ORG2_KEY_COUNT +
  1 + // suborg1
  SUBORG1_KEY_COUNT;
