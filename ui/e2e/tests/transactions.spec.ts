import { test, expect } from '@playwright/test';
import { formatReceiptId, formatTxHash } from '../helpers/format.js';
import { gotoTransactions } from '../helpers/navigation.js';
import { expectTransactionHashesForBlocks } from '../helpers/transactions.js';

test.describe('Transactions', () => {
  test.beforeEach(async ({ page }) => {
    await gotoTransactions(page);
  });

  test('Paginate transactions', async ({ page }) => {
    // Ensure all transactions are shown
    await page.getByRole('button', { name: 'All' }).click();

    // There should be 10 pages of 10 rows each
    for (let i = 1; i < 10; i++) {
      const next = 10 * i
      await expect(page.getByText(`${next} of more than ${next}`)).toBeVisible();
      await page.getByRole('button', { name: 'Go to next page' }).click();
    }
    await expect(page.getByText('100 of 100')).toBeVisible();
  });

  test('Paginate Paladin only transactions', async ({ page }) => {
    // There should be 5 pages of 10 rows each
    for (let i = 1; i < 5; i++) {
      const next = 10 * i
      await expect(page.getByText(`${next} of more than ${next}`)).toBeVisible();
      await page.getByRole('button', { name: 'Go to next page' }).click();
    }
    await expect(page.getByText('50 of 50')).toBeVisible();
  });

  test('Explore transaction', async ({ page }) => {
    // Navigate to fist transaction
    await page.getByRole('button', { name: 'Open' }).first().click();

    // Should navigate to transaction details with hash in URL
    await page.waitForURL(`**/ui/transactions/${formatTxHash(1)}`);
    await expect(page.getByRole('tab', { name: 'Pente0000...0001' })).toBeVisible();
  });

  test('Explore Paladin receipt', async ({ page }) => {
    // Click on a Paladin receipt
    await page.getByRole('button', { name: 'Pente 0000...0001' }).click();

    // Should navigate to transaction details with receipt UUID in URL
    await page.waitForURL('**/ui/transactions/00000000-0000-1000-8000-000000000001');
    await expect(page.getByRole('tab', { name: 'Pente0000...0001' })).toBeVisible();
  });

  test('Lookup transaction by hash', async ({ page }) => {
    // Use lookup dialog to enter hash
    await page.getByRole('button', { name: 'Lookup' }).click();
    await page.getByRole('textbox', { name: 'Blockchain Transaction Hash,' }).fill(formatTxHash(1));
    await page.getByRole('button', { name: 'Lookup' }).click();

    // Should navigate to transaction details with hash in URL
    await page.waitForURL(`**/ui/transactions/${formatTxHash(1)}`);
    await expect(page.getByRole('tab', { name: 'Pente0000...0001' })).toBeVisible();
  });

    test('Lookup transaction by receipt UUID', async ({ page }) => {
    // Use lookup dialog to enter hash
    await page.getByRole('button', { name: 'Lookup' }).click();
    await page.getByRole('textbox', { name: 'Blockchain Transaction Hash,' }).fill(formatReceiptId(1));
    await page.getByRole('button', { name: 'Lookup' }).click();

    // Should navigate to transaction details with hash in URL
    await page.waitForURL(`**/ui/transactions/${formatReceiptId(1)}`);
    await expect(page.getByRole('tab', { name: 'Pente0000...0001' })).toBeVisible();
  });

  test('Filter transactions', async ({ page }) => {
    // Add filter to show transactions with block number greater than 50
    await page.getByRole('button', { name: 'Filters', exact: true }).click();
    await page.getByRole('button', { name: 'Add Filter' }).click();
    await page.getByRole('combobox', { name: 'Field' }).click();
    await page.getByRole('option', { name: 'Block' }).click();
    await page.getByRole('combobox', { name: 'Operator' }).click();
    await page.getByRole('option', { name: 'Greater than', exact: true }).click();
    await page.getByRole('textbox', { name: 'Value' }).fill('50');
    await page.getByRole('button', { name: 'Add' }).click();

    // Add second filter to show transactions with block number less than or equal to 55
    await page.getByRole('button', { name: 'Filters', exact: true }).click();
    await page.getByRole('button', { name: 'Add Filter' }).click();
    await page.getByRole('combobox', { name: 'Field' }).click();
    await page.getByRole('option', { name: 'Block' }).click();
    await page.getByRole('combobox', { name: 'Operator' }).click();
    await page.getByRole('option', { name: 'Less than or equal', exact: true }).click();
    await page.getByRole('textbox', { name: 'Value' }).fill('55');
    await page.getByRole('button', { name: 'Add' }).click();

    // There should be exactly 2 transactions
    await expect(page.getByText('2 of 2')).toBeVisible();
  });

});



