import { expect, test } from '@playwright/test';
import { gotoStates } from '../helpers/navigation.js';
import { formatHex } from '../mock-server/fixtures/format-utils.js';
import { formatStateId } from '../helpers/format.js';

test.describe('States', () => {
  test.beforeEach(async ({ page }) => {
    await gotoStates(page);
  });

  test('List states', async ({ page }) => {

    // Show (up to) 100 rows per page
    await page.getByRole('combobox', { name: 'Rows per page:' }).click();
    await page.getByRole('option', { name: '100' }).click();

    // There should be 25 rows
    await expect(page.getByText('25 of 25')).toBeVisible();

    for (let i = 1; i <= 25; i++) {
      await expect(page.getByRole('cell', { name: (100 * i).toString(), exact: true })).toBeVisible();
    }
  });

  test('Explore state', async ({ page }) => {
    // Navigate to fist state
    await page.getByRole('button', { name: 'Open' }).first().click();

    // Should navigate to state details with hash in URL
    await page.waitForURL(`**/ui/states/noto/${formatHex(1, 64, '8')}/${formatHex(1, 64, '7')}`);
    await expect(page.getByRole('tab', { name: '0x70...0001' })).toBeVisible();

    // Should show an option to go back to states
    await expect(page.getByRole('button', { name: 'Back to States' })).toBeVisible();
  });

  test('State sorting', async ({ page }) => {
    // Apply sort using schema field "amount"
    await page.getByRole('button', { name: 'amount' }).click();

    // Default order
    await expect(page.getByRole('row').nth(1).getByRole('cell', { name: '2500' })).toBeVisible();
    await expect(page.getByRole('row').nth(10).getByRole('cell', { name: '1600' })).toBeVisible();

    // Reverse order
    await page.getByRole('button', { name: 'Created' }).click();

    // Check order is inverted
    await expect(page.getByRole('row').nth(1).getByRole('cell', { name: '100' })).toBeVisible();
    await expect(page.getByRole('row').nth(10).getByRole('cell', { name: '1000' })).toBeVisible();
  });

  test('Lookup state by ID', async ({ page }) => {
    // Use lookup dialog to enter hash
    await page.getByRole('button', { name: 'Lookup' }).click();
    await page.getByRole('textbox', { name: 'State ID' }).fill(formatStateId(1));
    await page.getByRole('button', { name: 'Lookup' }).click();

    // Should navigate to state details with hash in URL
    await page.waitForURL(`**/ui/states/noto/${formatHex(1, 64, '8')}/${formatHex(1, 64, '7')}`);
    await expect(page.getByRole('tab', { name: '0x70...0001' })).toBeVisible();

    // Should show an option to go back to states
    await expect(page.getByRole('button', { name: 'Back to States' })).toBeVisible();
  });

  test('Filter states', async ({ page }) => {
    // Add filter to show transactions with block number greater than 50
    await page.getByRole('button', { name: 'Filters', exact: true }).click();
    await page.getByRole('button', { name: 'Add Filter' }).click();
    await page.getByRole('combobox', { name: 'Field' }).click();
    await page.getByRole('option', { name: 'amount' }).click();
    await page.getByRole('combobox', { name: 'Operator' }).click();
    await page.getByRole('option', { name: 'Equal', exact: true }).click();
    await page.getByRole('textbox', { name: 'Value' }).fill('100');
    await page.getByRole('button', { name: 'Add' }).click();

    // There should be exactly 2 transactions
    await expect(page.getByText('1 of 1')).toBeVisible();
  });

  test('Switch schema and domain', async ({ page }) => {

    // Switch schema
    await page.getByRole('combobox', { name: 'NotoCoin owner, amount' }).click();
    await page.getByRole('option', { name: 'NotoLockedCoin owner, amount' }).click();

    // There should be a "locked" column
    await expect(page.getByRole('button', { name: 'locked' })).toBeVisible();

    // Switch to Pente domain (and as a consequence to AccountState schema)
    await page.getByRole('combobox', { name: 'Noto', exact: true }).click();
    await page.getByRole('option', { name: 'Pente' }).click();

    // There should be a "balance" column
    await expect(page.getByRole('button', { name: 'balance' })).toBeVisible();

  });


  test.describe('Edit actions for states', () => {

    test('Send status', async ({ page }) => {

      // Switch to "Edit" mode
      await page.locator('#settings').click();
      await page.locator('#editMode').click();
      await page.locator('.MuiBackdrop-root').click();

      // There should be an action to deploy
      await expect(page.getByRole('button', { name: 'Send' })).toHaveCount(10);
    });
  });


});
