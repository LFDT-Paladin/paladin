import { expect, type Page } from '@playwright/test';
import { getShortHash } from './format.js';

export const getTransactionCardLocator = (page: Page, blockNumber: number) =>
  page
    .getByRole('heading', { name: String(blockNumber), exact: true })
    .locator('../../../..');

export async function expectTransactionHashesForBlocks(
  page: Page,
  firstBlockNumber: number,
  lastBlockNumber: number,
  expectedFirstHash: string,
  expectedLastHash: string
) {
  await expect(
    page.getByRole('heading', { name: String(firstBlockNumber), exact: true })
  ).toBeVisible();
  await expect(
    page.getByRole('heading', { name: String(lastBlockNumber), exact: true })
  ).toBeVisible();

  const firstCard = getTransactionCardLocator(page, firstBlockNumber);
  const lastCard = getTransactionCardLocator(page, lastBlockNumber);

  await expect(
    firstCard.getByRole('button', { name: getShortHash(expectedFirstHash) }).first()
  ).toBeVisible();
  await expect(
    lastCard.getByRole('button', { name: getShortHash(expectedLastHash) }).first()
  ).toBeVisible();
}
