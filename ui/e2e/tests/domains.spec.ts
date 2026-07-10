import { test } from '@playwright/test';
import { gotoDomains } from '../helpers/navigation.js';

test.describe('Domains', () => {
  test.beforeEach(async ({ page }) => {
    await gotoDomains(page);
  });

  test.fixme('renders the domains panel correctly', async ({ page }) => {
    // TODO: assert heading, domain selector, and smart contracts table render from mock data
  });
});
