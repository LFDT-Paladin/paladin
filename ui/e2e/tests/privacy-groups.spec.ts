import { test } from '@playwright/test';
import {
  gotoPrivacyGroupListeners,
  gotoPrivacyGroupMessages,
  gotoPrivacyGroups,
} from '../helpers/navigation.js';

test.describe('Privacy Groups', () => {
  test.describe('Groups', () => {
    test.beforeEach(async ({ page }) => {
      await gotoPrivacyGroups(page);
    });

    test.fixme('renders the privacy groups panel correctly', async ({ page }) => {
      // TODO: assert heading, group table, and create button render from mock data
    });
  });

  test.describe('Messages', () => {
    test.beforeEach(async ({ page }) => {
      await gotoPrivacyGroupMessages(page);
    });

    test.fixme('renders the privacy group messages panel correctly', async ({ page }) => {
      // TODO: assert heading and messages table render from mock data
    });
  });

  test.describe('Listeners', () => {
    test.beforeEach(async ({ page }) => {
      await gotoPrivacyGroupListeners(page);
    });

    test.fixme('renders the privacy group listeners panel correctly', async ({ page }) => {
      // TODO: assert heading and listeners table render from mock data
    });
  });
});
