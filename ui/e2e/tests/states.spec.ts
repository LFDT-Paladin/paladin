import { test } from '@playwright/test';
import { gotoStates } from '../helpers/navigation.js';

test.describe('States', () => {
  test.beforeEach(async ({ page }) => {
    await gotoStates(page);
  });

  test.fixme('renders the states panel correctly', async ({ page }) => {
    // TODO: assert heading, domain/schema selectors, and state table render from mock data
  });
});
