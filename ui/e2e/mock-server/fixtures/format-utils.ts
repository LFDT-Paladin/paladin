// Copyright contributors to Paladin, an LFDT project
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/** Pad a number as a lowercase hex string to the given length. */
export const padHex = (value: number, length: number): string =>
  value.toString(16).padStart(length, '0');

/** Pad a number as a decimal string to the given length. */
export const padDecimal = (value: number, length: number): string =>
  value.toString(10).padStart(length, '0');

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
 * Format a deterministic UUID-like id: `00000000-0000-1000-8000-{leadingDigits}{padded n}`.
 * The last 12 characters are `leadingDigits` plus `n` as a zero-padded decimal.
 */
export const formatUuid = (n: number, leadingDigits = ''): string =>
  `00000000-0000-1000-8000-${leadingDigits}${padDecimal(n, 12 - leadingDigits.length)}`;
