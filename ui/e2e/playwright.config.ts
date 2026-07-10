import { defineConfig } from '@playwright/test';

const mockRpcPort = process.env.MOCK_RPC_PORT ?? '31999';
const mockRpcUrl = `http://localhost:${mockRpcPort}`;

export default defineConfig({
  testDir: './tests',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: 'list',
  use: {
    baseURL: 'http://localhost:3000',
    trace: 'on-first-retry',
  },
  webServer: [
    {
      command: 'npm run mock-server',
      url: `${mockRpcUrl}/health`,
      reuseExistingServer: !process.env.CI,
      env: {
        MOCK_RPC_PORT: mockRpcPort,
      },
    },
    {
      command: 'npm run dev:test',
      cwd: '../client',
      url: 'http://localhost:3000/ui/transactions',
      reuseExistingServer: !process.env.CI,
      env: {
        MOCK_RPC_PORT: mockRpcPort,
      },
    },
  ],
});
