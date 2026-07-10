import { test } from '@playwright/test';
import { gotoSubmissions } from '../helpers/navigation.js';

test.describe('Submissions', () => {
  test.beforeEach(async ({ page }) => {
    await gotoSubmissions(page);
  });

  test.fixme('renders the submissions panel correctly', async ({ page }) => {
    // TODO: assert heading, section toggles, and submission table/cards render from mock data
  });
});
