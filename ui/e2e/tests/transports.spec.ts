import { expect, test } from '@playwright/test';
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
