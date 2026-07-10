import { formatHex } from './format-utils.js';

export const REGISTRY_NAMES = ['default'] as const;
export type RegistryName = (typeof REGISTRY_NAMES)[number];

export const REGISTRY_ENTRY_COUNT = 25;
/** Last N entries are inactive. */
export const REGISTRY_INACTIVE_COUNT = 5;

/** Entry IDs are bytes32 hex values. */
export const formatRegistryEntryId = (n: number): string => formatHex(n, 64);
/** Owner addresses use leading nibble `1`. */
export const formatRegistryOwner = (n: number): string => formatHex(n, 40, '1');

export interface MockRegistry {
  name: RegistryName;
}

export interface MockRegistryEntry {
  registry: RegistryName;
  id: string;
  name: string;
  active: boolean;
  properties: {
    $owner: string;
    'transport.grpc'?: string;
  };
}

/**
 * Builds the configured identity registry.
 *
 * Name: default (returned as a string by reg_registries via listField).
 */
export const buildRegistries = (): MockRegistry[] =>
  REGISTRY_NAMES.map((name) => ({ name }));

const padNameIndex = (n: number): string => n.toString(10).padStart(2, '0');

/**
 * Builds registry entries for the Registries page.
 *
 * Layout (id/owner sequence n=1..25):
 * - entry01..entry20 → active
 * - entry21..entry25 → inactive
 *
 * Conventions:
 * - id:     0x...n (bytes32)
 * - $owner: 0x1...n (address)
 * - names:  zero-padded for predictable .name sort
 */
export const buildRegistryEntries = (): MockRegistryEntry[] => {
  const entries: MockRegistryEntry[] = [];

  for (let i = 0; i < REGISTRY_ENTRY_COUNT; i++) {
    const n = i + 1;
    const name = `entry${padNameIndex(n)}`;
    const active = n <= REGISTRY_ENTRY_COUNT - REGISTRY_INACTIVE_COUNT;
    entries.push({
      registry: 'default',
      id: formatRegistryEntryId(n),
      name,
      active,
      properties: {
        $owner: formatRegistryOwner(n),
        'transport.grpc': JSON.stringify({
          endpoint: `dns:///${name}.example:18888`,
        }),
      },
    });
  }

  return entries;
};
