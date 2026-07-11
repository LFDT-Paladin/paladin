import { expect, test } from '@playwright/test';
import { gotoRegistries } from '../helpers/navigation.js';
import { formatTxHash } from '../helpers/format.js';
import { formatAddress } from '../mock-server/fixtures/transaction-data.js';

test.describe('Registries', () => {
  test.beforeEach(async ({ page }) => {
    await gotoRegistries(page);
  });

  test('List all entries', async ({ page }) => {
    // Show 25 rows at a time
    await page.getByRole('combobox', { name: 'Rows per page:' }).click();
    await page.getByRole('option', { name: '25' }).click();

    // There should be exactly 25 rows
    await expect(page.getByText('25 of 25')).toBeVisible();
  });

  test('List active entries', async ({ page }) => {
    // Show 25 rows at a time
    await page.getByRole('combobox', { name: 'Rows per page:' }).click();
    await page.getByRole('option', { name: '25' }).click();

    // Show active entries
    await page.getByRole('button', { name: 'Active', exact: true }).click();

    // There should be exactly 20 rows
    await expect(page.getByText('20 of 20')).toBeVisible();
  });

  test('List inactive entries', async ({ page }) => {
    // Show 25 rows at a time
    await page.getByRole('combobox', { name: 'Rows per page:' }).click();
    await page.getByRole('option', { name: '25' }).click();

    // Show active entries
    await page.getByRole('button', { name: 'Inactive', exact: true }).click();

    // There should be exactly 5 rows
    await expect(page.getByText('5 of 5')).toBeVisible();
  });

  test('Explore entry', async ({ page }) => {
    // Click on entry
    await page.getByRole('row', { name: 'entry01 0x0000...0001 0x1000' }).getByLabel('Open').click();

    // Should navigate to transaction details with hash in URL
    await page.waitForURL(`**/ui/registries/default/${formatTxHash(1)}`);
    await expect(page.getByRole('tab', { name: '0x00...0001' })).toBeVisible();

    // Should show an option to go back to submissions
    await expect(page.getByRole('button', { name: 'Back to Registries' })).toBeVisible();
  });

  test('Resolve identifier', async ({ page }) => {
    // Open resolve dialog
    await page.getByRole('button', { name: 'Resolve' }).click();

    // Enter address
    await page.getByRole('textbox', { name: 'Key Identifier' }).fill(formatTxHash(1));
    await page.getByRole('button', { name: 'Resolve' }).click();

    // Should resolve
    await expect(page.getByText(formatAddress(1))).toBeVisible();

  });

  test('Filter registry entries', async ({ page }) => {
    // Add filter to show transactions with block number greater than 50
    await page.getByRole('button', { name: 'Filters', exact: true }).click();
    await page.getByRole('button', { name: 'Add Filter' }).click();
    await page.getByRole('combobox', { name: 'Field' }).click();
    await page.getByRole('option', { name: 'Name' }).click();
    await page.getByRole('combobox', { name: 'Operator' }).click();
    await page.getByRole('option', { name: 'Contains', exact: true }).click();
    await page.getByRole('textbox', { name: 'Value' }).fill('5');
    await page.getByRole('button', { name: 'Add' }).click();

    // There should be exactly 3 registry entries
    await expect(page.getByText('3 of 3')).toBeVisible();
  });

  test('Registry entries sorting', async ({ page }) => {
    // Default order
    await expect(page.getByRole('row').nth(1).getByRole('cell', { name: 'entry01', exact: true })).toBeVisible();
    await expect(page.getByRole('row').nth(10).getByRole('cell', { name: 'entry10', exact: true })).toBeVisible();

    // Apply sort
    await page.getByRole('button', { name: 'Name' }).click();

    // Check order is inverted
    await expect(page.getByRole('row').nth(1).getByRole('cell', { name: 'entry25', exact: true })).toBeVisible();
    await expect(page.getByRole('row').nth(10).getByRole('cell', { name: 'entry16', exact: true })).toBeVisible();
  });


});
