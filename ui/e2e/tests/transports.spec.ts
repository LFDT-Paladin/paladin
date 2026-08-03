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
import { formatReliableMessageId } from '../helpers/format.js';
import {
  gotoTransportConnections,
  gotoTransportMessages,
} from '../helpers/navigation.js';

test.describe('Transports', () => {

  test.describe('Connections', () => {

    test.beforeEach(async ({ page }) => {
      await gotoTransportConnections(page);
    });

    test('List connections', async ({ page }) => {
      // Show (up to) 100 rows per page
      await page.getByRole('combobox', { name: 'Rows per page:' }).click();
      await page.getByRole('option', { name: '100' }).click();

      // There should be 25 rows
      await expect(page.getByText('25 of 25')).toBeVisible();

      for (let i = 1; i <= 25; i++) {
        await expect(page.getByRole('cell', { name: `node${i.toString().padStart(2, '0')}`, exact: true })).toBeVisible();
      }
    });

    test('Sort connections', async ({ page }) => {
      // Default order
      await expect(page.getByRole('row').nth(1).getByRole('cell', { name: 'node01' })).toBeVisible();
      await expect(page.getByRole('row').nth(10).getByRole('cell', { name: 'node10' })).toBeVisible();

      // Reverse order
      await page.getByRole('button', { name: 'Node' }).click();

      // Check order is inverted
      await expect(page.getByRole('row').nth(1).getByRole('cell', { name: 'node25' })).toBeVisible();
      await expect(page.getByRole('row').nth(10).getByRole('cell', { name: 'node16' })).toBeVisible();
    });

    test('Filter connections', async ({ page }) => {
      // Add filter to show transactions with block number greater than 50
      await page.getByRole('button', { name: 'Filters', exact: true }).click();
      await page.getByRole('button', { name: 'Add Filter' }).click();
      await page.getByRole('combobox', { name: 'Field' }).click();
      await page.getByRole('option', { name: 'Node' }).click();
      await page.getByRole('combobox', { name: 'Operator' }).click();
      await page.getByRole('option', { name: 'Ends with', exact: true }).click();
      await page.getByRole('textbox', { name: 'Value' }).fill('5');
      await page.getByRole('button', { name: 'Add' }).click();

      // There should be exactly 2 transactions
      await expect(page.getByText('3 of 3')).toBeVisible();
    });

  });

  test.describe('Messages', () => {
    test.beforeEach(async ({ page }) => {
      await gotoTransportMessages(page);
    });

    test('List messages', async ({ page }) => {
      // Show (up to) 100 rows per page
      await page.getByRole('combobox', { name: 'Rows per page:' }).click();
      await page.getByRole('option', { name: '100' }).click();

      // There should be 25 rows
      await expect(page.getByText('50 of 50')).toBeVisible();

      for (let i = 1; i <= 25; i++) {
        await expect(page.getByRole('cell', { name: `node${i.toString().padStart(2, '0')}`, exact: true })).toHaveCount(2);
      }
    });

    test('Lookup transport message by ID', async ({ page }) => {
      // Use lookup dialog to enter message ID
      await page.getByRole('button', { name: 'Lookup' }).click();
      await page.getByRole('textbox', { name: 'Message ID' }).fill(formatReliableMessageId(1));
      await page.getByRole('button', { name: 'Lookup' }).click();

      // Should navigate to transport message details with id in URL
      await page.waitForURL(`**/ui/transports/messages/${formatReliableMessageId(1)}`);
      await expect(page.getByRole('tab', { name: '0000...0001' })).toBeVisible();

      // Should show an option to go back to transports
      await expect(page.getByRole('button', { name: 'Back to Transports' })).toBeVisible();
    });

    test('Sort messages', async ({ page }) => {
      // Default order
      await expect(page.getByRole('row').nth(1).getByRole('cell', { name: 'node01' })).toBeVisible();
      await expect(page.getByRole('row').nth(10).getByRole('cell', { name: 'node10' })).toBeVisible();

      // Reverse order
      await page.getByRole('button', { name: 'Created' }).click();

      // Check order is inverted
      await expect(page.getByRole('row').nth(1).getByRole('cell', { name: 'node25' })).toBeVisible();
      await expect(page.getByRole('row').nth(10).getByRole('cell', { name: 'node16' })).toBeVisible();
    });

    test('Filter messages', async ({ page }) => {
      // Add filter to show transactions with block number greater than 50
      await page.getByRole('button', { name: 'Filters', exact: true }).click();
      await page.getByRole('button', { name: 'Add Filter' }).click();
      await page.getByRole('combobox', { name: 'Field' }).click();
      await page.getByRole('option', { name: 'Node' }).click();
      await page.getByRole('combobox', { name: 'Operator' }).click();
      await page.getByRole('option', { name: 'Ends with', exact: true }).click();
      await page.getByRole('textbox', { name: 'Value' }).fill('5');
      await page.getByRole('button', { name: 'Add' }).click();

      // There should be exactly 2 transactions
      await expect(page.getByText('6 of 6')).toBeVisible();
    });

  });
});
