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

import type { Page } from '@playwright/test';

export const Routes = {
  Transactions: '/ui/transactions',
  Submissions: '/ui/submissions',
  Keys: '/ui/keys',
  Registries: '/ui/registries',
  Domains: '/ui/domains',
  PrivacyGroups: '/ui/privacy-groups/groups',
  PrivacyGroupMessages: '/ui/privacy-groups/messages',
  PrivacyGroupListeners: '/ui/listeners/privacy-groups',
  States: '/ui/states',
  TransportConnections: '/ui/transports/connections',
  TransportMessages: '/ui/transports/messages',
} as const;

export const gotoTransactions = (page: Page) =>
  page.goto(Routes.Transactions);

export const gotoSubmissions = (page: Page) =>
  page.goto(Routes.Submissions);

export const gotoKeys = (page: Page) =>
  page.goto(Routes.Keys);

export const gotoRegistries = (page: Page) =>
  page.goto(Routes.Registries);

export const gotoDomains = (page: Page) =>
  page.goto(Routes.Domains);

export const gotoPrivacyGroups = (page: Page) =>
  page.goto(Routes.PrivacyGroups);

export const gotoPrivacyGroupMessages = (page: Page) =>
  page.goto(Routes.PrivacyGroupMessages);

export const gotoPrivacyGroupListeners = (page: Page) =>
  page.goto(Routes.PrivacyGroupListeners);

export const gotoStates = (page: Page) =>
  page.goto(Routes.States);

export const gotoTransportConnections = (page: Page) =>
  page.goto(Routes.TransportConnections);

export const gotoTransportMessages = (page: Page) =>
  page.goto(Routes.TransportMessages);
