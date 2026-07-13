import { expect, test } from '@playwright/test';
import {
  gotoPrivacyGroupListeners,
  gotoPrivacyGroupMessages,
  gotoPrivacyGroups,
} from '../helpers/navigation.js';
import { formatHex } from '../mock-server/fixtures/format-utils.js';
import { formatMessageId } from '../helpers/format.js';

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

      // Should navigate to privact group details with hash in URL
      await page.waitForURL(`**/ui/privacy-groups/groups/${formatHex(1, 64, 'e')}`);
      await expect(page.getByRole('tab', { name: 'group01 0xe0...0001' })).toBeVisible();

      // Should show an option to go back to submissions
      await expect(page.getByRole('button', { name: 'Back to Privacy Groups' })).toBeVisible();
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
      // Navigate to fist privacy group
      await page.getByRole('button', { name: 'Open' }).first().click();

      // Should navigate to privact group details with hash in URL
      await page.waitForURL(`**/ui/privacy-groups/messages/${formatMessageId(1)}`);
      await expect(page.getByRole('tab', { name: '0000...0001' })).toBeVisible();

      // Should show an option to go back to submissions
      await expect(page.getByRole('button', { name: 'Back to Messages' })).toBeVisible();
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

  test.describe('Listeners', () => {
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

      // Should navigate to privact group details with hash in URL
      await page.waitForURL('**/ui/privacy-groups/listeners/listener01');
      await expect(page.getByRole('tab', { name: 'listener01' })).toBeVisible();

      // Should show an option to go back to listeners
      await expect(page.getByRole('button', { name: 'Back to Listeners' })).toBeVisible();
    });

    test.describe('Edit actions for listeners', () => {

      test('Send message', async ({ page }) => {

        // Switch to "Edit" mode
        await page.locator('#settings').click();
        await page.locator('#editMode').click();
        await page.locator('.MuiBackdrop-root').click();

        // Should show an option to go back to create new listeners
        await expect(page.getByRole('button', { name: 'Create', exact: true })).toBeVisible();
      });
    });

    test('Start/stop/delete', async ({ page }) => {

      // Switch to "Edit" mode
      await page.locator('#settings').click();
      await page.locator('#editMode').click();
      await page.locator('.MuiBackdrop-root').click();

      // There should be an actions to start, stop and delete
      await expect(page.getByText('StartStopDelete')).toHaveCount(10);
    });

  });
});
