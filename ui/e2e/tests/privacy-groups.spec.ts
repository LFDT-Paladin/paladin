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
  gotoPrivacyGroupMessages,
  gotoPrivacyGroups,
} from '../helpers/navigation.js';
import {
  formatMessageId,
  formatPrivacyGroupAddress,
  formatPrivacyGroupId,
} from '../helpers/format.js';

test.describe('Privacy Groups', () => {
  test.describe('Groups', () => {

    test.beforeEach(async ({ page }) => {
      await gotoPrivacyGroups(page);
    });

    test('List groups', async ({ page }) => {

      // Show (up to) 100 rows per page
      await page.getByRole('combobox', { name: 'Rows per page:' }).click();
      await page.getByRole('option', { name: '100' }).click();

      // There should be 25 rows
      await expect(page.getByText('25 of 25')).toBeVisible();

      for (let i = 1; i <= 25; i++) {
        await expect(page.getByRole('cell', { name: `group${i.toString().padStart(2, '0')}` })).toBeVisible();
      }
    });

    test('Explore group', async ({ page }) => {
      // Navigate to fist privacy group
      await page.getByRole('button', { name: 'Open' }).first().click();

      // Should navigate to privacy group details with id in URL
      await page.waitForURL(`**/ui/privacy-groups/groups/${formatPrivacyGroupId(1)}`);
      await expect(page.getByRole('tab', { name: 'group01 0xe0...0001' })).toBeVisible();

      // Should show an option to go back to privacy groups
      await expect(page.getByRole('button', { name: 'Back to Privacy Groups' })).toBeVisible();
    });

    test('Lookup privacy group by ID', async ({ page }) => {
      // Use lookup dialog to enter privacy group ID
      await page.getByRole('button', { name: 'Lookup' }).click();
      await page.getByRole('textbox', { name: 'Privacy Group ID or Contract Address' }).fill(formatPrivacyGroupId(1));
      await page.getByRole('button', { name: 'Lookup' }).click();

      // Should navigate to privacy group details with id in URL
      await page.waitForURL(`**/ui/privacy-groups/groups/${formatPrivacyGroupId(1)}`);
      await expect(page.getByRole('tab', { name: 'group01 0xe0...0001' })).toBeVisible();

      // Should show an option to go back to privacy groups
      await expect(page.getByRole('button', { name: 'Back to Privacy Groups' })).toBeVisible();
    });

    test('Lookup privacy group by contract address', async ({ page }) => {
      // Use lookup dialog to enter contract address
      await page.getByRole('button', { name: 'Lookup' }).click();
      await page.getByRole('textbox', { name: 'Privacy Group ID or Contract Address' }).fill(formatPrivacyGroupAddress(1));
      await page.getByRole('button', { name: 'Lookup' }).click();

      // Should navigate to privacy group details with address in URL
      await page.waitForURL(`**/ui/privacy-groups/groups/${formatPrivacyGroupAddress(1)}`);
      await expect(page.getByRole('tab', { name: 'group01 0xe0...0001' })).toBeVisible();

      // Should show an option to go back to privacy groups
      await expect(page.getByRole('button', { name: 'Back to Privacy Groups' })).toBeVisible();
    });

    test('Privacy group sorting', async ({ page }) => {
      // Default order
      await expect(page.getByRole('row').nth(1).getByRole('button', { name: '0xe000...0001' })).toBeVisible();
      await expect(page.getByRole('row').nth(10).getByRole('button', { name: '0xe000...000a' })).toBeVisible();

      // Apply sort
      await page.getByRole('button', { name: 'Created' }).click();

      // Check order is inverted
      await expect(page.getByRole('row').nth(1).getByRole('button', { name: '0xe000...0019' })).toBeVisible();
      await expect(page.getByRole('row').nth(10).getByRole('button', { name: '0xe000...0010' })).toBeVisible();
    });

  });

  test.describe('Messages', () => {
    test.beforeEach(async ({ page }) => {
      await gotoPrivacyGroupMessages(page);
    });

    test('List messages', async ({ page }) => {

      // Show (up to) 100 rows per page
      await page.getByRole('combobox', { name: 'Rows per page:' }).click();
      await page.getByRole('option', { name: '100' }).click();

      // There should be 50 rows
      await expect(page.getByText('50 of 50')).toBeVisible();

      for (let i = 1; i <= 50; i++) {
        await expect(page.getByRole('button', { name: `000000...00${i.toString().padStart(2, '0')}` })).toBeVisible();
      }
    });

    test('Explore message', async ({ page }) => {
      // Navigate to fist privacy group message
      await page.getByRole('button', { name: 'Open' }).first().click();

      // Should navigate to privacy group message details with id in URL
      await page.waitForURL(`**/ui/privacy-groups/messages/${formatMessageId(1)}`);
      await expect(page.getByRole('tab', { name: '0000...0001' })).toBeVisible();

      // Should show an option to go back to messages
      await expect(page.getByRole('button', { name: 'Back to Messages' })).toBeVisible();
    });

    test('Lookup privacy group message by ID', async ({ page }) => {
      // Use lookup dialog to enter message ID
      await page.getByRole('button', { name: 'Lookup' }).click();
      await page.getByRole('textbox', { name: 'Message ID' }).fill(formatMessageId(1));
      await page.getByRole('button', { name: 'Lookup' }).click();

      // Should navigate to privacy group message details with id in URL
      await page.waitForURL(`**/ui/privacy-groups/messages/${formatMessageId(1)}`);
      await expect(page.getByRole('tab', { name: '0000...0001' })).toBeVisible();

      // Should show an option to go back to messages
      await expect(page.getByRole('button', { name: 'Back to Messages' })).toBeVisible();
    });

    test('Privacy group message sorting', async ({ page }) => {
      // Default order
      await expect(page.getByRole('row').nth(1).getByRole('button', { name: '0000...0001' })).toBeVisible();
      await expect(page.getByRole('row').nth(10).getByRole('button', { name: '0000...0010' })).toBeVisible();

      // Apply sort
      await page.getByRole('button', { name: 'Sent' }).click();

      // Check order is inverted
      await expect(page.getByRole('row').nth(1).getByRole('button', { name: '00000...0050' })).toBeVisible();
      await expect(page.getByRole('row').nth(10).getByRole('button', { name: '00000...0041' })).toBeVisible();
    });

    test.describe('Edit actions for messages', () => {

      test('Send message', async ({ page }) => {

        // Switch to "Edit" mode
        await page.locator('#settings').click();
        await page.locator('#editMode').click();
        await page.locator('.MuiBackdrop-root').click();

        // There should be an action to deploy
        await expect(page.getByRole('button', { name: 'Send' })).toBeVisible();
      });
    });

  });

});
