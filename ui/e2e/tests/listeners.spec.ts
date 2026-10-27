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
import {
  gotoEventListeners,
  gotoPrivacyGroupListeners,
  gotoReceiptListeners,
} from '../helpers/navigation.js';

test.describe('Listeners', () => {

  test.describe('Event listeners', () => {
    test.beforeEach(async ({ page }) => {
      await gotoEventListeners(page);
    });

    test('List listeners', async ({ page }) => {

      // Show (up to) 100 rows per page
      await page.getByRole('combobox', { name: 'Rows per page:' }).click();
      await page.getByRole('option', { name: '100' }).click();

      // There should be 12 rows
      await expect(page.getByText('12 of 12')).toBeVisible();

      for (let i = 1; i <= 12; i++) {
        await expect(page.getByRole('cell', { name: `eventlistener${i.toString().padStart(2, '0')}` })).toBeVisible();
      }
    });

    test('Explore listener', async ({ page }) => {
      // Navigate to fist privacy group
      await page.getByRole('button', { name: 'Open' }).first().click();

      // Should navigate to privacy group details with hash in URL
      await page.waitForURL('**/ui/listeners/events/eventlistener01');
      await expect(page.getByRole('tab', { name: 'eventlistener01' })).toBeVisible();

      // Should show an option to go back to listeners
      await expect(page.getByRole('button', { name: 'Back to Listeners' })).toBeVisible();
    });

    test('Privacy group listener sorting', async ({ page }) => {
      // Default order
      await expect(page.getByRole('row').nth(1).getByRole('cell', { name: 'listener01' })).toBeVisible();
      await expect(page.getByRole('row').nth(10).getByRole('cell', { name: 'listener10' })).toBeVisible();

      // Apply sort
      await page.getByRole('button', { name: 'name' }).click();

      // Check order is inverted
      await expect(page.getByRole('row').nth(1).getByRole('cell', { name: 'listener12' })).toBeVisible();
      await expect(page.getByRole('row').nth(10).getByRole('cell', { name: 'listener03' })).toBeVisible();
    });

    test.describe('Edit actions for listeners', () => {

      test('Start / Stop / Delete', async ({ page }) => {

        // Switch to "Edit" mode
        await page.locator('#settings').click();
        await page.locator('#editMode').click();
        await page.locator('.MuiBackdrop-root').click();

        // There should be an actions to start, stop and delete
        await expect(page.getByText('StartStopDelete')).toHaveCount(10);
      });
    });
  });

  test.describe('Receipt listeners', () => {
    test.beforeEach(async ({ page }) => {
      await gotoReceiptListeners(page);
    });

    test('List listeners', async ({ page }) => {

      // Show (up to) 100 rows per page
      await page.getByRole('combobox', { name: 'Rows per page:' }).click();
      await page.getByRole('option', { name: '100' }).click();

      // There should be 12 rows
      await expect(page.getByText('12 of 12')).toBeVisible();

      for (let i = 1; i <= 12; i++) {
        await expect(page.getByRole('cell', { name: `receiptlistener${i.toString().padStart(2, '0')}` })).toBeVisible();
      }
    });

    test('Explore listener', async ({ page }) => {
      // Navigate to fist privacy group
      await page.getByRole('button', { name: 'Open' }).first().click();

      // Should navigate to privacy group details with hash in URL
      await page.waitForURL('**/ui/listeners/receipts/receiptlistener01');
      await expect(page.getByRole('tab', { name: 'receiptlistener01' })).toBeVisible();

      // Should show an option to go back to listeners
      await expect(page.getByRole('button', { name: 'Back to Listeners' })).toBeVisible();
    });

    test('Privacy group listener sorting', async ({ page }) => {
      // Default order
      await expect(page.getByRole('row').nth(1).getByRole('cell', { name: 'receiptlistener01' })).toBeVisible();
      await expect(page.getByRole('row').nth(10).getByRole('cell', { name: 'receiptlistener10' })).toBeVisible();

      // Apply sort
      await page.getByRole('button', { name: 'name' }).click();

      // Check order is inverted
      await expect(page.getByRole('row').nth(1).getByRole('cell', { name: 'receiptlistener12' })).toBeVisible();
      await expect(page.getByRole('row').nth(10).getByRole('cell', { name: 'receiptlistener03' })).toBeVisible();
    });

    test.describe('Edit actions for listeners', () => {

      test('Start / Stop / Delete', async ({ page }) => {

        // Switch to "Edit" mode
        await page.locator('#settings').click();
        await page.locator('#editMode').click();
        await page.locator('.MuiBackdrop-root').click();

        // There should be an actions to start, stop and delete
        await expect(page.getByText('StartStopDelete')).toHaveCount(10);
      });
    });
  });


  test.describe('Privacy group listeners', () => {
    test.beforeEach(async ({ page }) => {
      await gotoPrivacyGroupListeners(page);
    });

    test('List listeners', async ({ page }) => {

      // Show (up to) 100 rows per page
      await page.getByRole('combobox', { name: 'Rows per page:' }).click();
      await page.getByRole('option', { name: '100' }).click();

      // There should be 12 rows
      await expect(page.getByText('12 of 12')).toBeVisible();

      for (let i = 1; i <= 12; i++) {
        await expect(page.getByRole('cell', { name: `listener${i.toString().padStart(2, '0')}` })).toBeVisible();
      }
    });

    test('Explore listener', async ({ page }) => {
      // Navigate to fist privacy group
      await page.getByRole('button', { name: 'Open' }).first().click();

      // Should navigate to privacy group details with hash in URL
      await page.waitForURL('**/ui/listeners/privacy-groups/listener01');
      await expect(page.getByRole('tab', { name: 'listener01' })).toBeVisible();

      // Should show an option to go back to listeners
      await expect(page.getByRole('button', { name: 'Back to Listeners' })).toBeVisible();
    });

    test('Privacy group listener sorting', async ({ page }) => {
      // Default order
      await expect(page.getByRole('row').nth(1).getByRole('cell', { name: 'listener01' })).toBeVisible();
      await expect(page.getByRole('row').nth(10).getByRole('cell', { name: 'listener10' })).toBeVisible();

      // Apply sort
      await page.getByRole('button', { name: 'name' }).click();

      // Check order is inverted
      await expect(page.getByRole('row').nth(1).getByRole('cell', { name: 'listener12' })).toBeVisible();
      await expect(page.getByRole('row').nth(10).getByRole('cell', { name: 'listener03' })).toBeVisible();
    });

    test.describe('Edit actions for listeners', () => {

      test('Start / Stop / Delete', async ({ page }) => {

        // Switch to "Edit" mode
        await page.locator('#settings').click();
        await page.locator('#editMode').click();
        await page.locator('.MuiBackdrop-root').click();

        // There should be an actions to start, stop and delete
        await expect(page.getByText('StartStopDelete')).toHaveCount(10);
      });
    });
  });

});
