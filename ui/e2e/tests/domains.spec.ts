import { expect, test } from '@playwright/test';
import { gotoDomains } from '../helpers/navigation.js';
import { formatHex } from '../mock-server/fixtures/format-utils.js';
import { formatAddress } from '../mock-server/fixtures/transaction-data.js';

test.describe('Domains', () => {
  test.beforeEach(async ({ page }) => {
    await gotoDomains(page);
  });

  test('Noto smart contracts', async ({ page }) => {

    // Noto should be selected by default
    await expect(page.getByRole('combobox', { name: 'Noto' })).toBeVisible();

    // There should be 10 entries
    await expect(page.getByText('10 of 10')).toBeVisible();

    // There should be columns "Symbol" and "Is Notary"
    await expect(page.getByRole('columnheader', { name: 'Symbol' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Is Notary' })).toBeVisible();

    // There should be an action to check the balance of each entry
    await expect(page.getByRole('button', { name: 'Balance' })).toHaveCount(10);

  });

  test('Pente smart contracts', async ({ page }) => {

    // Switch to Pente
    await page.getByRole('combobox', { name: 'Noto' }).click();
    await page.getByRole('option', { name: 'Pente' }).click();

    // There should be 10 entries
    await expect(page.getByText('10 of 10')).toBeVisible();

    // There should be no actions
    await expect(page.getByText('No actions')).toHaveCount(10);
  });

  test('Zeto smart contracts', async ({ page }) => {

    // Switch to Pente
    await page.getByRole('combobox', { name: 'Noto' }).click();
    await page.getByRole('option', { name: 'Zeto' }).click();

    // There should be 10 entries
    await expect(page.getByText('10 of 10')).toBeVisible();

    // There should be an action to check the balance of each entry
    await expect(page.getByRole('button', { name: 'Balance' })).toHaveCount(10);
  });

  test('Smart contract sorting', async ({ page }) => {
    // Default order
    await expect(page.getByRole('row').nth(1)).toContainText('0x0000...0001');
    await expect(page.getByRole('row').nth(10)).toContainText('0x0000...000a');

    // Apply sort
    await page.getByRole('button', { name: 'Deployed' }).click();

    // Check order is inverted
    await expect(page.getByRole('row').nth(1)).toContainText('0x0000...000a');
    await expect(page.getByRole('row').nth(10)).toContainText('0x0000...0001');
  });

  test('Smart contract details', async ({ page }) => {
    // Navigate to smart contract
    await page.getByRole('button', { name: 'Open' }).first().click();

    // Should navigate to smart contract details with hash in URL
    await page.waitForURL(`**/ui/domains/${formatHex(1, 40)}`);
    await expect(page.getByRole('tab', { name: 'Noto0x00...0001' })).toBeVisible();

    // Should show an option to go back to domains
    await expect(page.getByRole('button', { name: 'Back to Domains' })).toBeVisible();
  });

  test('Lookup smart contract', async ({ page }) => {
    // Use lookup dialog to enter smart contract address
    await page.getByRole('button', { name: 'Lookup' }).click();
    await page.getByRole('textbox', { name: 'Contract Address' }).fill(formatAddress(1));
    await page.getByRole('button', { name: 'Lookup' }).click();

    // Should navigate to smart contract details with address in URL
    await page.waitForURL(`**/ui/domains/${formatAddress(1)}`);
    await expect(page.getByRole('tab', { name: 'Noto0x00...0001' })).toBeVisible();

    // Should show an option to go back to domains
    await expect(page.getByRole('button', { name: 'Back to Domains' })).toBeVisible();
  });

  test('Filter smart contracts', async ({ page }) => {
    // Add filter to show a specific smart contract
    await page.getByRole('button', { name: 'Filters', exact: true }).click();
    await page.getByRole('button', { name: 'Add Filter' }).click();
    await page.getByRole('combobox', { name: 'Field' }).click();
    await page.getByRole('option', { name: 'Contract Address' }).click();
    await page.getByRole('combobox', { name: 'Operator' }).click();
    await page.getByRole('option', { name: 'Equal', exact: true }).click();
    await page.getByRole('textbox', { name: 'Value' }).fill(formatAddress(1));
    await page.getByRole('button', { name: 'Add' }).click();

    // There should be exactly 1 smart contract
    await expect(page.getByText('1 of 1')).toBeVisible();
  });


  test.describe('Edit actions for Domains', () => {

    test('Noto: deploy, mint, transfer and burn', async ({ page }) => {
      // Switch to "Edit" mode
      await page.locator('#settings').click();
      await page.locator('#editMode').click();
      await page.locator('.MuiBackdrop-root').click();

      // There should be an action to deploy
      await expect(page.getByRole('button', { name: 'Deploy new' })).toBeVisible();

      // There should be actions for mint, transfer and burn
      await expect(page.getByRole('button', { name: 'Mint' })).toHaveCount(10);
      await expect(page.getByRole('button', { name: 'Transfer' })).toHaveCount(10);
      await expect(page.getByRole('button', { name: 'Burn' })).toHaveCount(10);

    });


    test('Pente: no actions', async ({ page }) => {
      // Switch to "Edit" mode
      await page.locator('#settings').click();
      await page.locator('#editMode').click();
      await page.locator('.MuiBackdrop-root').click();

      // Switch to Pente
      await page.getByRole('combobox', { name: 'Noto' }).click();
      await page.getByRole('option', { name: 'Pente' }).click();

      // There should be no actions
      await expect(page.getByText('No actions')).toHaveCount(10);

      // The action to deploy should be disabled
      await expect(page.getByRole('button', { name: 'Deploy new' })).toBeDisabled();
    });

    test('Zeto: deploy, mint and transfer', async ({ page }) => {
      // Switch to "Edit" mode
      await page.locator('#settings').click();
      await page.locator('#editMode').click();
      await page.locator('.MuiBackdrop-root').click();

      // Switch to Zeto
      await page.getByRole('combobox', { name: 'Noto' }).click();
      await page.getByRole('option', { name: 'Zeto' }).click();

      // There should be an action to deploy
      await expect(page.getByRole('button', { name: 'Deploy new' })).toBeVisible();

      // There should be actions for mint, transfer and burn
      await expect(page.getByRole('button', { name: 'Mint' })).toHaveCount(10);
      await expect(page.getByRole('button', { name: 'Transfer' })).toHaveCount(10);
    });

  });


});
