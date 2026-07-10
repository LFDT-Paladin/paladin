import type { Page } from '@playwright/test';

export const Routes = {
  Transactions: '/ui/transactions',
  Submissions: '/ui/submissions',
  Keys: '/ui/keys',
  Registries: '/ui/registries',
  Domains: '/ui/domains',
  PrivacyGroups: '/ui/privacy-groups/groups',
  PrivacyGroupMessages: '/ui/privacy-groups/messages',
  PrivacyGroupListeners: '/ui/privacy-groups/listeners',
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
