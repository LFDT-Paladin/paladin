// Copyright contributors to Paladin, an LFDT project
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
