import { test } from '@playwright/test';
import {
  gotoTransportConnections,
  gotoTransportMessages,
} from '../helpers/navigation.js';

test.describe('Transports', () => {
  test.describe('Connections', () => {
    test.beforeEach(async ({ page }) => {
      await gotoTransportConnections(page);
    });

    test.fixme('renders the transport connections panel correctly', async ({ page }) => {
      // TODO: assert heading and peer connections table render from mock data
    });
  });

  test.describe('Messages', () => {
    test.beforeEach(async ({ page }) => {
      await gotoTransportMessages(page);
    });

    test.fixme('renders the transport messages panel correctly', async ({ page }) => {
      // TODO: assert heading and reliable messages table render from mock data
    });
  });
});
