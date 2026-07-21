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

import { generatePath } from 'react-router-dom';

export const AppRoutes = {
  Keys: '/ui/keys',
  Submissions: '/ui/submissions',
  Registries: '/ui/registries',
  Domains: '/ui/domains',
  Transactions: '/ui/transactions',
  Transaction: '/ui/transactions/:hashOrId',
  DomainContract: '/ui/domains/:address',
  PrivacyGroups: '/ui/privacy-groups/groups',
  PrivacyGroup: '/ui/privacy-groups/groups/:idOrAddress',
  PrivacyGroupMessages: '/ui/privacy-groups/messages',
  States: '/ui/states',
  ReliableMessage: '/ui/transports/messages/:id',
  State: '/ui/states/:domain/:schema/:id',
  RegistryEntry: '/ui/registries/:registry/:id',
  PrivacyGroupMessageEntry: '/ui/privacy-groups/messages/:messageId',
  TransportConnections: '/ui/transports/connections',
  TransportMessages: '/ui/transports/messages',
  EventListeners: '/ui/listeners/events',
  EventListenerEntry: '/ui/listeners/events/:id',
  ReceiptListeners: '/ui/listeners/receipts',
  ReceiptListenerEntry: '/ui/listeners/receipts/:id',
  PrivacyGroupListeners: '/ui/listeners/privacy-groups',
  PrivacyGroupListenerEntry: '/ui/listeners/privacy-groups/:name',
} as const;

export type AppRouteName = keyof typeof AppRoutes;

export const AppRouteFactory = {
  getPath(
    route: AppRouteName,
    params: Record<string, string | number> = {},
    query?: Record<string, string>
  ): string {
    // Params are route-specific; the dynamic route key prevents generatePath from narrowing them.
    const path = generatePath(AppRoutes[route], params as never);
    if (query === undefined) {
      return path;
    }
    const searchParams = new URLSearchParams(query);
    const qs = searchParams.toString();
    return qs ? `${path}?${qs}` : path;
  },
};
