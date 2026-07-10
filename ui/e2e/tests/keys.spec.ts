import { test } from '@playwright/test';
import { gotoKeys } from '../helpers/navigation.js';

test.describe('Keys', () => {
  test.beforeEach(async ({ page }) => {
    await gotoKeys(page);
  });

  test.fixme('renders the keys panel correctly', async ({ page }) => {
    // TODO: assert heading, mode toggles, and key table renders from mock data
  });
});
