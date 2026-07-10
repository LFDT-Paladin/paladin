/** Pad a number as a lowercase hex string to the given length. */
export const padHex = (value: number, length: number): string =>
  value.toString(16).padStart(length, '0');

/**
 * Format a 0x-prefixed hex value.
 * @param n - numeric sequence value
 * @param totalHexLength - number of hex digits after `0x` (e.g. 40 for address, 64 for hash)
 * @param leadingNibble - optional leading hex digit(s) that reduce the padded width of `n`
 */
export const formatHex = (
  n: number,
  totalHexLength: number,
  leadingNibble = ''
): string => `0x${leadingNibble}${padHex(n, totalHexLength - leadingNibble.length)}`;

/**
 * Format a deterministic UUID-like id: `00000000-0000-1000-8000-{leadingNibble}{padded n}`.
 * The last 12 characters are `leadingNibble` plus `n` padded to fill the remainder.
 */
export const formatUuid = (n: number, leadingNibble = ''): string =>
  `00000000-0000-1000-8000-${leadingNibble}${padHex(n, 12 - leadingNibble.length)}`;
