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

import { expect, type Page } from '@playwright/test';
import { getShortHash } from './format.js';

export const getTransactionCardLocator = (page: Page, blockNumber: number) =>
  page
    .getByRole('heading', { name: String(blockNumber), exact: true })
    .locator('../../../..');

export async function expectTransactionHashesForBlocks(
  page: Page,
  firstBlockNumber: number,
  lastBlockNumber: number,
  expectedFirstHash: string,
  expectedLastHash: string
) {
  await expect(
    page.getByRole('heading', { name: String(firstBlockNumber), exact: true })
  ).toBeVisible();
  await expect(
    page.getByRole('heading', { name: String(lastBlockNumber), exact: true })
  ).toBeVisible();

  const firstCard = getTransactionCardLocator(page, firstBlockNumber);
  const lastCard = getTransactionCardLocator(page, lastBlockNumber);

  await expect(
    firstCard.getByRole('button', { name: getShortHash(expectedFirstHash) }).first()
  ).toBeVisible();
  await expect(
    lastCard.getByRole('button', { name: getShortHash(expectedLastHash) }).first()
  ).toBeVisible();
}
