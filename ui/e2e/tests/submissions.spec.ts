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
import { gotoSubmissions } from '../helpers/navigation.js';
import { formatReceiptId, formatTxHash } from '../helpers/format.js';

test.describe('Submissions', () => {
  test.beforeEach(async ({ page }) => {
    await gotoSubmissions(page);
  });

  test('Paginate pending transactions', async ({ page }) => {
    // There should be 3 pages, 10 rows in the first 2 and 5 in the last
    for (let i = 1; i < 3; i++) {
      const next = 10 * i
      await expect(page.getByText(`${next} of more than ${next}`)).toBeVisible();
      await page.getByRole('button', { name: 'Go to next page' }).click();
    }
    await expect(page.getByText('25 of 25')).toBeVisible();
  });

  test('Paginate failed transactions', async ({ page }) => {
    // Ensure all transactions are shown
    await page.getByRole('button', { name: 'Failed' }).click();

    // There should be 1 page with 5 rows
    await expect(page.getByText('5 of 5')).toBeVisible();
  });

  test('Explore transaction', async ({ page }) => {
    // Navigate to fist transaction
    await page.getByRole('button', { name: 'Open' }).first().click();

    // Should navigate to transaction details with hash in URL
    await page.waitForURL(`**/ui/transactions/${formatReceiptId(1)}?back=submissions`);
    await expect(page.getByRole('tab', { name: 'Pente0000...0001' })).toBeVisible();

    // Should show an option to go back to submissions
    await expect(page.getByRole('button', { name: 'Back to Submissions' })).toBeVisible();
  });

  test('Lookup pending transaction by UUID', async ({ page }) => {
    // Use lookup dialog to enter hash
    await page.getByRole('button', { name: 'Lookup' }).click();
    await page.getByRole('textbox', { name: 'Submission ID' }).fill(formatReceiptId(1));
    await page.getByRole('button', { name: 'Lookup' }).click();

    // Should navigate to transaction details with hash in URL
    await page.waitForURL(`**/ui/transactions/${formatReceiptId(1)}?back=submissions`);
    await expect(page.getByRole('tab', { name: 'Pente0000...0001' })).toBeVisible();

    // Should show an option to go back to submissions
    await expect(page.getByRole('button', { name: 'Back to Submissions' })).toBeVisible();
  });

  test('Filter submissions', async ({ page }) => {
    // Add filter to show transactions with block number greater than 50
    await page.getByRole('button', { name: 'Filters', exact: true }).click();
    await page.getByRole('button', { name: 'Add Filter' }).click();
    await page.getByRole('combobox', { name: 'Field' }).click();
    await page.getByRole('option', { name: 'Domain' }).click();
    await page.getByRole('combobox', { name: 'Operator' }).click();
    await page.getByRole('option', { name: 'Equal', exact: true }).click();
    await page.getByRole('textbox', { name: 'Value' }).fill('zeto');
    await page.getByRole('button', { name: 'Add' }).click();

    // There should be no submissions
    await expect(page.getByText('No submissions')).toBeVisible();

  });

  test('Submissions sorting', async ({ page }) => {
    // Default order
    await expect(page.getByRole('row').nth(1).getByRole('button', { name: '000000...0001' })).toBeVisible();
    await expect(page.getByRole('row').nth(10).getByRole('button', { name: '000000...0010' })).toBeVisible();

    // Apply sort
    await page.getByRole('button', { name: 'Created' }).click();

    // Check order is inverted
    await expect(page.getByRole('row').nth(1).getByRole('button', { name: '000000...0025' })).toBeVisible();
    await expect(page.getByRole('row').nth(10).getByRole('button', { name: '000000...0016' })).toBeVisible();
  });

});
