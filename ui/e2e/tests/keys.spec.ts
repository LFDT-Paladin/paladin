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

import { expect, test } from '@playwright/test';
import { gotoKeys } from '../helpers/navigation.js';

test.describe('Keys', () => {
  test.beforeEach(async ({ page }) => {
    await gotoKeys(page);
  });

  test('List mode view', async ({ page }) => {
    // Show (up to) 100 rows per page
    await page.getByRole('combobox', { name: 'Rows per page:' }).click();
    await page.getByRole('option', { name: '100' }).click();

    // There should be 32 rows
    await expect(page.getByText('23 of 23')).toBeVisible();
  });

  test('Explorer mode view', async ({ page }) => {
    // Switch to Explorer view
    await page.getByRole('button', { name: 'Explorer View' }).click();

    // There should be 7 rows
    await expect(page.getByText('7 of 7')).toBeVisible();
  });


  test('Navigate folders', async ({ page }) => {
    // Switch to Explorer view
    await page.getByRole('button', { name: 'Explorer View' }).click();

    // Verify root keys
    for (let i = 1; i <= 5; i++) {
      await expect(page.getByRole('cell', { name: `rootkey${i}` })).toBeVisible();
    }

    // Expand org1
    await page.getByRole('row', { name: 'Open folder org1' }).getByLabel('Open folder').click();

    // Verify org1 keys
    for (let i = 1; i <= 5; i++) {
      await expect(page.getByRole('cell', { name: `org1key${i}` })).toBeVisible();
    }

    // Navigate to org2
    await page.getByRole('button', { name: 'Open folder' }).click();

    // Verify org1 keys
    for (let i = 1; i <= 5; i++) {
      await expect(page.getByRole('cell', { name: `suborg1key${i}` })).toBeVisible();
    }

    // Navigate back to root
    await page.getByRole('link', { name: 'Root', exact: true }).click();

    // Expand org2
    await page.getByRole('row', { name: 'Open folder org2' }).getByLabel('Open folder').click();

    // Verify org2 keys
    for (let i = 1; i <= 5; i++) {
      await expect(page.getByRole('cell', { name: `org2key${i}` })).toBeVisible();
    }
  });

  test('Filter keys', async ({ page }) => {
    // Apply filter to show entries that are of type folder and key
    await page.getByRole('button', { name: 'Filters' }).click();
    await page.getByRole('button', { name: 'Add Filter' }).click();
    await page.getByRole('combobox', { name: 'Field' }).click();
    await page.getByRole('option', { name: 'Is folder' }).click();
    await page.getByRole('combobox', { name: 'Operator' }).click();
    await page.getByRole('option', { name: 'Equal' }).click();
    await page.getByRole('combobox', { name: 'Value' }).click();
    await page.getByRole('option', { name: 'True' }).click();
    await page.getByRole('button', { name: 'Add' }).click();
    await page.getByRole('button', { name: 'Add Filter' }).click();
    await page.getByRole('combobox', { name: 'Field' }).click();
    await page.getByRole('option', { name: 'Is key' }).click();
    await page.getByRole('combobox', { name: 'Operator' }).click();
    await page.getByRole('option', { name: 'Equal' }).click();
    await page.getByRole('combobox', { name: 'Value' }).click();
    await page.getByRole('option', { name: 'True' }).click();
    await page.getByRole('button', { name: 'Add' }).click();

    // There should be 1 row
    await expect(page.getByText('1 of 1')).toBeVisible();
  });

  test('Keys sorting', async ({ page }) => {
    // Show (up to) 100 rows per page
    await page.getByRole('combobox', { name: 'Rows per page:' }).click();
    await page.getByRole('option', { name: '100' }).click();

    // Default order
    await expect(page.getByRole('row').nth(1).getByRole('cell', { name: 'org1', exact: true })).toBeVisible();
    await expect(page.getByRole('row').nth(23).getByRole('cell', { name: 'rootkey5', exact: true })).toBeVisible();

    // Apply sort
    await page.getByRole('button', { name: 'Path' }).click();

    // Check order is inverted
    await expect(page.getByRole('row').nth(1).getByRole('cell', { name: 'rootkey5', exact: true })).toBeVisible();
    await expect(page.getByRole('row').nth(23).getByRole('cell', { name: 'org1', exact: true })).toBeVisible();
  });

});
