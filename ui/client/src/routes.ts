// Copyright © 2026 Kaleido, Inc.
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

export const AppRoutes = {
  Keys: '/ui/keys',
  Submissions: '/ui/submissions',
  Registry: '/ui/registry',
  Domains: '/ui/domains',
  Transactions: '/ui/transactions',
  Transaction: '/ui/transactions/:hashOrId',
  DomainContract: '/ui/domains/:address',
  PrivacyGroups: '/ui/privacy-groups',
  PrivacyGroup: '/ui/privacy-groups/:idOrAddress',
  States: '/ui/states',
  ReliableMessage: '/ui/transports/messages/:id',
  State: '/ui/states/:domain/:schema/:id',
  RegistryEntry: '/ui/registry/:registry/:id',
  PrivacyGroupMessageEntry: '/ui/privacy-groups/:privacyGroupId/messages/:messageId',
  TransportConnections: '/ui/transports/connections',
  TransportMessages: '/ui/transports/messages'
};
