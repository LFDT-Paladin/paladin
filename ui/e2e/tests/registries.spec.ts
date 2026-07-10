import { test } from '@playwright/test';
import { gotoRegistries } from '../helpers/navigation.js';

test.describe('Registries', () => {
  test.beforeEach(async ({ page }) => {
    await gotoRegistries(page);
  });

  test.fixme('renders the registries panel correctly', async ({ page }) => {
    // TODO: assert heading, registry selector, and entry table renders from mock data
  });
});
