import { formatHex } from './format-utils.js';

export const DOMAIN_NAMES = ['noto', 'pente', 'zeto'] as const;
export type DomainName = (typeof DOMAIN_NAMES)[number];

export const NOTO_CONTRACT_COUNT = 25;
export const ZETO_CONTRACT_COUNT = 15;
export const PENTE_CONTRACT_COUNT = 5;
export const SMART_CONTRACT_COUNT =
  NOTO_CONTRACT_COUNT + ZETO_CONTRACT_COUNT + PENTE_CONTRACT_COUNT;

/** Registry addresses use leading nibble `c` (distinct from tx/key fixtures). */
export const formatNotoRegistryAddress = (): string => formatHex(1, 40, 'c');
export const formatPenteRegistryAddress = (): string => formatHex(2, 40, 'c');
export const formatZetoRegistryAddress = (): string => formatHex(3, 40, 'c');

/** Contract addresses use leading nibble `d` with a global sequence across domains. */
export const formatContractAddress = (n: number): string => formatHex(n, 40, 'd');

export const registryAddressForDomain = (name: DomainName): string => {
  switch (name) {
    case 'noto':
      return formatNotoRegistryAddress();
    case 'pente':
      return formatPenteRegistryAddress();
    case 'zeto':
      return formatZetoRegistryAddress();
  }
};

export interface MockDomain {
  name: DomainName;
  registryAddress: string;
  config: {
    signingAlgorithms: Record<string, number>;
  };
}

export interface MockDomainContract {
  domainName: DomainName;
  domainAddress: string;
  address: string;
  created: string;
  config: {
    contractConfig: Record<string, unknown>;
  };
}

/**
 * Builds the three configured domains (noto, pente, zeto).
 *
 * Conventions:
 * - registryAddress: 0xc...0001 (noto), 0xc...0002 (pente), 0xc...0003 (zeto)
 */
export const buildDomains = (): MockDomain[] =>
  DOMAIN_NAMES.map((name) => ({
    name,
    registryAddress: registryAddressForDomain(name),
    config: {
      signingAlgorithms: {
        'ecdsa:secp256k1': 1,
      },
    },
  }));

const buildContract = (
  domainName: DomainName,
  sequence: number,
  localIndex: number,
  contractConfig: Record<string, unknown>
): MockDomainContract => ({
  domainName,
  domainAddress: registryAddressForDomain(domainName),
  address: formatContractAddress(sequence),
  // Newest first within and across domains: sequence 1 is most recent
  created: new Date(Date.UTC(2026, 0, 1, 12, 0, 0, 1000 - sequence)).toISOString(),
  config: { contractConfig },
});

/**
 * Builds smart contracts for the Domains page.
 *
 * Layout (global address sequence, newest-first by created):
 * - n=1..25  → noto  (NotoToken{i} / NT{i}, isNotary on odd i)
 * - n=26..40 → zeto  (ZetoToken{i})
 * - n=41..45 → pente (empty contractConfig)
 *
 * Conventions:
 * - address:        0xd...n
 * - domainAddress:  matching domain registryAddress (0xc...)
 * - created:        descending from 2026-01-01T12:00:00Z
 */
export const buildSmartContracts = (): MockDomainContract[] => {
  const contracts: MockDomainContract[] = [];
  let sequence = 1;

  for (let i = 0; i < NOTO_CONTRACT_COUNT; i++) {
    const local = i + 1;
    contracts.push(
      buildContract('noto', sequence++, local, {
        name: `NotoToken${local}`,
        symbol: `NT${local}`,
        isNotary: local % 2 === 1,
      })
    );
  }

  for (let i = 0; i < ZETO_CONTRACT_COUNT; i++) {
    const local = i + 1;
    contracts.push(
      buildContract('zeto', sequence++, local, {
        tokenName: `ZetoToken${local}`,
      })
    );
  }

  for (let i = 0; i < PENTE_CONTRACT_COUNT; i++) {
    const local = i + 1;
    contracts.push(buildContract('pente', sequence++, local, {}));
  }

  return contracts;
};
