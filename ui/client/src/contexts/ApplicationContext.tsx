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

import {
  createContext,
  Dispatch,
  SetStateAction,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import {
  IFilter,
  IPaladinTransactionPagingReference,
  IPrivacyGroupPagingReference,
  ISortPagingReference,
  IStatePagingReference,
  ITransactionPagingReference,
} from "../interfaces";
import { constants } from "../components/config";

export interface TransactionsViewState {
  refEntries: ITransactionPagingReference[];
  setRefEntries: Dispatch<SetStateAction<ITransactionPagingReference[]>>;
  page: number;
  setPage: Dispatch<SetStateAction<number>>;
  rowsPerPage: number;
  setRowsPerPage: Dispatch<SetStateAction<number>>;
  showTxsWithReceipt: boolean;
  setShowTxsWithReceipt: Dispatch<SetStateAction<boolean>>;
  filters: IFilter[];
  setFilters: Dispatch<SetStateAction<IFilter[]>>;
  filtersVisible: boolean;
  setFiltersVisible: Dispatch<SetStateAction<boolean>>;
}

export interface SubmissionsViewState {
  section: "pending" | "failed";
  setSection: Dispatch<SetStateAction<"pending" | "failed">>;
  page: number;
  setPage: Dispatch<SetStateAction<number>>;
  rowsPerPage: number;
  setRowsPerPage: Dispatch<SetStateAction<number>>;
  refEntries: IPaladinTransactionPagingReference[];
  setRefEntries: Dispatch<SetStateAction<IPaladinTransactionPagingReference[]>>;
  sortAscending: boolean;
  setSortAscending: Dispatch<SetStateAction<boolean>>;
  filters: IFilter[];
  setFilters: Dispatch<SetStateAction<IFilter[]>>;
  filtersVisible: boolean;
  setFiltersVisible: Dispatch<SetStateAction<boolean>>;
}

export interface DomainsViewState {
  sortAscending: boolean;
  setSortAscending: Dispatch<SetStateAction<boolean>>;
  page: number;
  setPage: Dispatch<SetStateAction<number>>;
  rowsPerPage: number;
  setRowsPerPage: Dispatch<SetStateAction<number>>;
  refEntries: ISortPagingReference[];
  setRefEntries: Dispatch<SetStateAction<ISortPagingReference[]>>;
  selectedDomain: string | undefined;
  setSelectedDomain: Dispatch<SetStateAction<string | undefined>>;
  filters: IFilter[];
  setFilters: Dispatch<SetStateAction<IFilter[]>>;
  filtersVisible: boolean;
  setFiltersVisible: Dispatch<SetStateAction<boolean>>;
}

export interface PrivacyGroupsViewState {
  sortAscending: boolean;
  setSortAscending: Dispatch<SetStateAction<boolean>>;
  sortBy: string;
  setSortBy: Dispatch<SetStateAction<string>>;
  refEntries: IPrivacyGroupPagingReference[];
  setRefEntries: Dispatch<SetStateAction<IPrivacyGroupPagingReference[]>>;
  page: number;
  setPage: Dispatch<SetStateAction<number>>;
  rowsPerPage: number;
  setRowsPerPage: Dispatch<SetStateAction<number>>;
  filters: IFilter[];
  setFilters: Dispatch<SetStateAction<IFilter[]>>;
  filtersVisible: boolean;
  setFiltersVisible: Dispatch<SetStateAction<boolean>>;
}

export interface PrivacyGroupListenersViewState {
  sortAscending: boolean;
  setSortAscending: Dispatch<SetStateAction<boolean>>;
  refEntries: ISortPagingReference[];
  setRefEntries: Dispatch<SetStateAction<ISortPagingReference[]>>;
  sortBy: string;
  setSortBy: Dispatch<SetStateAction<string>>;
  page: number;
  setPage: Dispatch<SetStateAction<number>>;
  rowsPerPage: number;
  setRowsPerPage: Dispatch<SetStateAction<number>>;
  filters: IFilter[];
  setFilters: Dispatch<SetStateAction<IFilter[]>>;
  filtersVisible: boolean;
  setFiltersVisible: Dispatch<SetStateAction<boolean>>;
}

export interface EventListenersViewState {
  sortAscending: boolean;
  setSortAscending: Dispatch<SetStateAction<boolean>>;
  refEntries: ISortPagingReference[];
  setRefEntries: Dispatch<SetStateAction<ISortPagingReference[]>>;
  sortBy: string;
  setSortBy: Dispatch<SetStateAction<string>>;
  page: number;
  setPage: Dispatch<SetStateAction<number>>;
  rowsPerPage: number;
  setRowsPerPage: Dispatch<SetStateAction<number>>;
  filters: IFilter[];
  setFilters: Dispatch<SetStateAction<IFilter[]>>;
  filtersVisible: boolean;
  setFiltersVisible: Dispatch<SetStateAction<boolean>>;
}

export interface StatesViewState {
  selectedDomain: string | undefined;
  setSelectedDomain: Dispatch<SetStateAction<string | undefined>>;
  selectedSchemaId: string | undefined;
  setSelectedSchemaId: Dispatch<SetStateAction<string | undefined>>;
  sortAscending: boolean;
  setSortAscending: Dispatch<SetStateAction<boolean>>;
  refEntries: IStatePagingReference[];
  setRefEntries: Dispatch<SetStateAction<IStatePagingReference[]>>;
  page: number;
  setPage: Dispatch<SetStateAction<number>>;
  rowsPerPage: number;
  setRowsPerPage: Dispatch<SetStateAction<number>>;
  filters: IFilter[];
  setFilters: Dispatch<SetStateAction<IFilter[]>>;
  filtersVisible: boolean;
  setFiltersVisible: Dispatch<SetStateAction<boolean>>;
}

export interface MessagesViewState {
  sortAscending: boolean;
  setSortAscending: Dispatch<SetStateAction<boolean>>;
  page: number;
  setPage: Dispatch<SetStateAction<number>>;
  rowsPerPage: number;
  setRowsPerPage: Dispatch<SetStateAction<number>>;
  refEntries: ISortPagingReference[];
  setRefEntries: Dispatch<SetStateAction<ISortPagingReference[]>>;
  filters: IFilter[];
  setFilters: Dispatch<SetStateAction<IFilter[]>>;
  sortBy: string;
  setSortBy: Dispatch<SetStateAction<string>>;
  filtersVisible: boolean;
  setFiltersVisible: Dispatch<SetStateAction<boolean>>;
}

export interface TransportsViewState {
  sortAscending: boolean;
  setSortAscending: Dispatch<SetStateAction<boolean>>;
  page: number;
  setPage: Dispatch<SetStateAction<number>>;
  rowsPerPage: number;
  setRowsPerPage: Dispatch<SetStateAction<number>>;
  refNames: string[];
  setRefNames: Dispatch<SetStateAction<string[]>>;
  filters: IFilter[];
  setFilters: Dispatch<SetStateAction<IFilter[]>>;
  filtersVisible: boolean;
  setFiltersVisible: Dispatch<SetStateAction<boolean>>;
}

export interface PrivacyGroupMessagesViewState {
  sortAscending: boolean;
  setSortAscending: Dispatch<SetStateAction<boolean>>;
  page: number;
  setPage: Dispatch<SetStateAction<number>>;
  rowsPerPage: number;
  setRowsPerPage: Dispatch<SetStateAction<number>>;
  refEntries: ISortPagingReference[];
  setRefEntries: Dispatch<SetStateAction<ISortPagingReference[]>>;
  filters: IFilter[];
  setFilters: Dispatch<SetStateAction<IFilter[]>>;
  filtersVisible: boolean;
  setFiltersVisible: Dispatch<SetStateAction<boolean>>;
}

export interface RegistryViewState {
  filters: IFilter[];
  setFilters: Dispatch<SetStateAction<IFilter[]>>;
  refNames: string[];
  setRefNames: Dispatch<SetStateAction<string[]>>;
  sortAscending: boolean;
  setSortAscending: Dispatch<SetStateAction<boolean>>;
  page: number;
  setPage: Dispatch<SetStateAction<number>>;
  rowsPerPage: number;
  setRowsPerPage: Dispatch<SetStateAction<number>>;
  filtersVisible: boolean;
  setFiltersVisible: Dispatch<SetStateAction<boolean>>;
}

export interface KeysViewState {
  page: number;
  setPage: Dispatch<SetStateAction<number>>;
  rowsPerPage: number;
  setRowsPerPage: Dispatch<SetStateAction<number>>;
  mode: "explorer" | "list";
  setMode: Dispatch<SetStateAction<"explorer" | "list">>;
  filters: IFilter[];
  setFilters: Dispatch<SetStateAction<IFilter[]>>;
  sortAscending: boolean;
  setSortAscending: Dispatch<SetStateAction<boolean>>;
  sortByPathFirst: boolean;
  setSortByPathFirst: Dispatch<SetStateAction<boolean>>;
  filtersVisible: boolean;
  setFiltersVisible: Dispatch<SetStateAction<boolean>>;
}

interface IApplicationContext {
  colorMode: {
    toggleColorMode: () => void;
  };
  navigationVisible: boolean;
  setNavigationVisible: Dispatch<SetStateAction<boolean>>;
  readOnly: boolean;
  setReadOnly: Dispatch<SetStateAction<boolean>>;
  transactions: TransactionsViewState;
  submissions: SubmissionsViewState;
  domains: DomainsViewState;
  privacyGroups: PrivacyGroupsViewState;
  privacyGroupListeners: PrivacyGroupListenersViewState;
  privacyGroupMessages: PrivacyGroupMessagesViewState;
  eventListeners: EventListenersViewState;
  states: StatesViewState;
  messages: MessagesViewState;
  transports: TransportsViewState;
  registry: RegistryViewState;
  keys: KeysViewState;
}

export const ApplicationContext = createContext({} as IApplicationContext);

export const useApplicationContext = () => useContext(ApplicationContext);

interface Props {
  colorMode: {
    toggleColorMode: () => void;
  };
  children: JSX.Element;
}

const getReadOnlyModeFromLocalStorage = () => {
  const value = window.localStorage.getItem(constants.MODE_STORAGE_KEY);
  return constants.EDIT_MODE_STORAGE_VALUE !== value;
}

export const ApplicationContextProvider = ({ children, colorMode }: Props) => {

  const [navigationVisible, setNavigationVisible] = useState(false);
  const [readOnly, setReadOnly] = useState(getReadOnlyModeFromLocalStorage());

  useEffect(() => {
    if(readOnly === true) {
      window.localStorage.removeItem(constants.MODE_STORAGE_KEY);
    } else {
      window.localStorage.setItem(constants.MODE_STORAGE_KEY, constants.EDIT_MODE_STORAGE_VALUE);
    }
  }, [readOnly]);

  // Transactions view state
  const [txRefEntries, setTxRefEntries] = useState<ITransactionPagingReference[]>([]);
  const [txPage, setTxPage] = useState(0);
  const [txRowsPerPage, setTxRowsPerPage] = useState(10);
  const [txShowTxsWithReceipt, setTxShowTxsWithReceipt] = useState(true);
  const [txFilters, setTxFilters] = useState<IFilter[]>([]);
  const [txFiltersVisible, setTxFiltersVisible] = useState(false);

  // Submissions view state
  const [submissionsSection, setSubmissionsSection] = useState<"pending" | "failed">("pending");
  const [submissionsRefEntries, setSubmissionsRefEntries] = useState<IPaladinTransactionPagingReference[]>([]);
  const [submissionsPage, setSubmissionsPage] = useState(0);
  const [submissionsRowsPerPage, setSubmissionsRowsPerPage] = useState(10);
  const [submissionsSortAscending, setSubmissionsSortAscending] = useState(false);
  const [submissionsFilters, setSubmissionsFilters] = useState<IFilter[]>([]);
  const [submissionsFiltersVisible, setSubmissionsFiltersVisible] = useState(false);

  // Domains view state
  const [domainSortAscending, setDomainSortAscending] = useState(false);
  const [domainsPage, setDomainsPage] = useState(0);
  const [domainsFilters, setDomainsFilters] = useState<IFilter[]>([]);
  const [domainsRowsPerPage, setDomainsRowsPerPage] = useState(10);
  const [domainsRefEntries, setDomainsRefEntries] = useState<ISortPagingReference[]>([]);
  const [domainsSelectedDomain, setDomainsSelectedDomain] = useState<string | undefined>();
  const [domainsFiltersVisible, setDomainsFiltersVisible] = useState(false);

  // Privacy groups view state
  const [privacyGroupsFilters, setPrivacyGroupsFilters] = useState<IFilter[]>([]);
  const [privacyGroupsPage, setPrivacyGroupsPage] = useState(0);
  const [privacyGroupsRowsPerPage, setPrivacyGroupsRowsPerPage] = useState(10);
  const [privacyGroupsRefEntries, setPrivacyGroupsRefEntries] = useState<IPrivacyGroupPagingReference[]>([]);
  const [privacyGroupsSortBy, setPrivacyGroupsSortBy] = useState('created');
  const [privacyGroupsSortAscending, setPrivacyGroupsSortAscending] = useState(false);
  const [privacyGroupsFiltersVisible, setPrivacyGroupsFiltersVisible] = useState(false);

  // Privacy group listeners view state
  const [privacyGroupListenersFilters, setPrivacyGroupListenersFilters] = useState<IFilter[]>([]);
  const [privacyGroupListenersPage, setPrivacyGroupListenersPage] = useState(0);
  const [privacyGroupListenersRowsPerPage, setPrivacyGroupListenersRowsPerPage] = useState(10);
  const [privacyGroupListenersRefEntries, setPrivacyGroupListenersRefEntries] = useState<ISortPagingReference[]>([]);
  const [privacyGroupListenersSortBy, setPrivacyGroupListenerssortBy] = useState('name');
  const [privacyGroupListenersSortAscending, setPrivacyGroupListenersSortAscending] = useState(true);
  const [privacyGroupListenersFiltersVisible, setPrivacyGroupListenersFiltersVisible] = useState(false);

  // Event listeners view state
  const [eventListenersFilters, setEventListenersFilters] = useState<IFilter[]>([]);
  const [eventListenersPage, setEventListenersPage] = useState(0);
  const [eventListenersRowsPerPage, setEventListenersRowsPerPage] = useState(10);
  const [eventListenersRefEntries, setEventListenersRefEntries] = useState<ISortPagingReference[]>([]);
  const [eventListenersSortBy, setEventListenersSortBy] = useState('name');
  const [eventListenersSortAscending, setEventListenersSortAscending] = useState(true);
  const [eventListenersFiltersVisible, setEventListenersFiltersVisible] = useState(false);

  // States view state
  const [statesSelectedDomain, setStatesSelectedDomain] = useState<string | undefined>();
  const [statesSelectedSchemaId, setStatesSelectedSchemaId] = useState<string | undefined>();
  const [statePage, setStatePage] = useState(0);
  const [stateRowsPerPage, setStateRowsPerPage] = useState(10);
  const [stateRefEntries, setStateRefEntries] = useState<IStatePagingReference[]>([]);
  const [stateSortAscending, setStateSortAscending] = useState(false);
  const [stateFilters, setStateFilters] = useState<IFilter[]>([]);
  const [stateFiltersVisible, setStateFiltersVisible] = useState(false);

  // Messages view state
  const [messagesPage, setMessagesPage] = useState(0);
  const [messagesRowsPerPage, setMessagesRowsPerPage] = useState(10);
  const [messagesRefEntries, setMessagesRefEntries] = useState<ISortPagingReference[]>([]);
  const [messagesSortAscending, setMessagesSortAscending] = useState(false);
  const [messagesFilters, setMessagesFilters] = useState<IFilter[]>([]);
  const [messagesSortBy, setMessagesSortBy] = useState("created");
  const [messagesFiltersVisible, setMessagesFiltersVisible] = useState(false);

  // Transports view state
  const [transportsPage, setTransportsPage] = useState(0);
  const [transportsRowsPerPage, setTransportsRowsPerPage] = useState(10);
  const [transportsRefNames, setTransportsRefNames] = useState<string[]>([]);
  const [transportsSortAscending, setTransportsSortAscending] = useState(true);
  const [transportsFilters, setTransportsFilters] = useState<IFilter[]>([]);
  const [transportsFiltersVisible, setTransportsFiltersVisible] = useState(false);

  // Privacy Group Messages view state
  const [privacyGroupMessagesPage, setPrivacyGroupMessagesPage] = useState(0);
  const [privacyGroupMessagesRowsPerPage, setPrivacyGroupMessagesRowsPerPage] = useState(10);
  const [privacyGroupMessagesRefEntries, setPrivacyGroupMessagesRefEntries] = useState<ISortPagingReference[]>([]);
  const [privacyGroupMessagesSortAscending, setPrivacyGroupMessagesSortAscending] = useState(false);
  const [privacyGroupMessagesFilters, setPrivacyGroupMessagesFilters] = useState<IFilter[]>([]);
  const [privacyGroupMessagesFiltersVisible, setPrivacyGroupMessagesFiltersVisible] = useState(false);

  // Registry view state
  const [registryFilters, setRegistryFilters] = useState<IFilter[]>([]);
  const [registryRefNames, setRegistryRefNames] = useState<string[]>([]);
  const [registrySortAscending, setRegistrySortAscending] = useState(true);
  const [registryPage, setRegistryPage] = useState(0);
  const [registryRowsPerPage, setRegistryRowsPerPage] = useState(10);
  const [registryFiltersVisible, setRegistryFiltersVisible] = useState(false);

  // Keys view state
  const [keysPage, setKeysPage] = useState(0);
  const [keysRowsPerPage, setKeysRowsPerPage] = useState(10);
  const [keysMode, setKeysMode] = useState<"explorer" | "list">("list");
  const [keysFilters, setKeysFilters] = useState<IFilter[]>([]);
  const [keysSortAscending, setKeysSortAscending] = useState(true);
  const [keysSortByPathFirst, setKeysSortByPathFirst] = useState(true);
  const [keysFiltersVisible, setKeysFiltersVisible] = useState(false);

  const transactions = useMemo(
    (): TransactionsViewState => ({
      refEntries: txRefEntries,
      setRefEntries: setTxRefEntries,
      page: txPage,
      setPage: setTxPage,
      rowsPerPage: txRowsPerPage,
      setRowsPerPage: setTxRowsPerPage,
      showTxsWithReceipt: txShowTxsWithReceipt,
      setShowTxsWithReceipt: setTxShowTxsWithReceipt,
      filters: txFilters,
      setFilters: setTxFilters,
      filtersVisible: txFiltersVisible,
      setFiltersVisible: setTxFiltersVisible,
    }),
    [
      txRefEntries,
      txPage,
      txRowsPerPage,
      txShowTxsWithReceipt,
      txFilters,
      txFiltersVisible,
    ]
  );

  const submissions = useMemo(
    (): SubmissionsViewState => ({
      section: submissionsSection,
      setSection: setSubmissionsSection,
      page: submissionsPage,
      setPage: setSubmissionsPage,
      rowsPerPage: submissionsRowsPerPage,
      setRowsPerPage: setSubmissionsRowsPerPage,
      refEntries: submissionsRefEntries,
      setRefEntries: setSubmissionsRefEntries,
      sortAscending: submissionsSortAscending,
      setSortAscending: setSubmissionsSortAscending,
      filters: submissionsFilters,
      setFilters: setSubmissionsFilters,
      filtersVisible: submissionsFiltersVisible,
      setFiltersVisible: setSubmissionsFiltersVisible,
    }),
    [
      submissionsSection,
      submissionsPage,
      submissionsRowsPerPage,
      submissionsRefEntries,
      submissionsSortAscending,
      submissionsFilters,
      submissionsFiltersVisible,
    ]
  );

  const domains = useMemo(
    (): DomainsViewState => ({
      sortAscending: domainSortAscending,
      setSortAscending: setDomainSortAscending,
      page: domainsPage,
      setPage: setDomainsPage,
      rowsPerPage: domainsRowsPerPage,
      setRowsPerPage: setDomainsRowsPerPage,
      refEntries: domainsRefEntries,
      setRefEntries: setDomainsRefEntries,
      selectedDomain: domainsSelectedDomain,
      setSelectedDomain: setDomainsSelectedDomain,
      filters: domainsFilters,
      setFilters: setDomainsFilters,
      filtersVisible: domainsFiltersVisible,
      setFiltersVisible: setDomainsFiltersVisible,
    }),
    [
      domainSortAscending,
      domainsPage,
      domainsRowsPerPage,
      domainsRefEntries,
      domainsSelectedDomain,
      domainsFilters,
      domainsFiltersVisible,
    ]
  );

  const privacyGroups = useMemo(
    (): PrivacyGroupsViewState => ({
      sortAscending: privacyGroupsSortAscending,
      setSortAscending: setPrivacyGroupsSortAscending,
      sortBy: privacyGroupsSortBy,
      setSortBy: setPrivacyGroupsSortBy,
      refEntries: privacyGroupsRefEntries,
      setRefEntries: setPrivacyGroupsRefEntries,
      page: privacyGroupsPage,
      setPage: setPrivacyGroupsPage,
      rowsPerPage: privacyGroupsRowsPerPage,
      setRowsPerPage: setPrivacyGroupsRowsPerPage,
      filters: privacyGroupsFilters,
      setFilters: setPrivacyGroupsFilters,
      filtersVisible: privacyGroupsFiltersVisible,
      setFiltersVisible: setPrivacyGroupsFiltersVisible,
    }),
    [
      privacyGroupsSortAscending,
      privacyGroupsSortBy,
      privacyGroupsRefEntries,
      privacyGroupsPage,
      privacyGroupsRowsPerPage,
      privacyGroupsFilters,
      privacyGroupsFiltersVisible,
    ]
  );

  const privacyGroupListeners = useMemo(
    (): PrivacyGroupListenersViewState => ({
      sortAscending: privacyGroupListenersSortAscending,
      setSortAscending: setPrivacyGroupListenersSortAscending,
      refEntries: privacyGroupListenersRefEntries,
      setRefEntries: setPrivacyGroupListenersRefEntries,
      sortBy: privacyGroupListenersSortBy,
      setSortBy: setPrivacyGroupListenerssortBy,
      page: privacyGroupListenersPage,
      setPage: setPrivacyGroupListenersPage,
      rowsPerPage: privacyGroupListenersRowsPerPage,
      setRowsPerPage: setPrivacyGroupListenersRowsPerPage,
      filters: privacyGroupListenersFilters,
      setFilters: setPrivacyGroupListenersFilters,
      filtersVisible: privacyGroupListenersFiltersVisible,
      setFiltersVisible: setPrivacyGroupListenersFiltersVisible,
    }),
    [
      privacyGroupListenersSortAscending,
      privacyGroupListenersRefEntries,
      privacyGroupListenersSortBy,
      privacyGroupListenersPage,
      privacyGroupListenersRowsPerPage,
      privacyGroupListenersFilters,
      privacyGroupListenersFiltersVisible,
    ]
  );

  const eventListeners = useMemo(
    (): EventListenersViewState => ({
      sortAscending: eventListenersSortAscending,
      setSortAscending: setEventListenersSortAscending,
      refEntries: eventListenersRefEntries,
      setRefEntries: setEventListenersRefEntries,
      sortBy: eventListenersSortBy,
      setSortBy: setEventListenersSortBy,
      page: eventListenersPage,
      setPage: setEventListenersPage,
      rowsPerPage: eventListenersRowsPerPage,
      setRowsPerPage: setEventListenersRowsPerPage,
      filters: eventListenersFilters,
      setFilters: setEventListenersFilters,
      filtersVisible: eventListenersFiltersVisible,
      setFiltersVisible: setEventListenersFiltersVisible,
    }),
    [
      eventListenersSortAscending,
      eventListenersRefEntries,
      eventListenersSortBy,
      eventListenersPage,
      eventListenersRowsPerPage,
      eventListenersFilters,
      eventListenersFiltersVisible,
    ]
  );

  const states = useMemo(
    (): StatesViewState => ({
      selectedDomain: statesSelectedDomain,
      setSelectedDomain: setStatesSelectedDomain,
      selectedSchemaId: statesSelectedSchemaId,
      setSelectedSchemaId: setStatesSelectedSchemaId,
      sortAscending: stateSortAscending,
      setSortAscending: setStateSortAscending,
      refEntries: stateRefEntries,
      setRefEntries: setStateRefEntries,
      page: statePage,
      setPage: setStatePage,
      rowsPerPage: stateRowsPerPage,
      setRowsPerPage: setStateRowsPerPage,
      filters: stateFilters,
      setFilters: setStateFilters,
      filtersVisible: stateFiltersVisible,
      setFiltersVisible: setStateFiltersVisible,
    }),
    [
      statesSelectedDomain,
      statesSelectedSchemaId,
      stateSortAscending,
      stateRefEntries,
      statePage,
      stateRowsPerPage,
      stateFilters,
      stateFiltersVisible,
    ]
  );

  const messages = useMemo(
    (): MessagesViewState => ({
      sortAscending: messagesSortAscending,
      setSortAscending: setMessagesSortAscending,
      page: messagesPage,
      setPage: setMessagesPage,
      rowsPerPage: messagesRowsPerPage,
      setRowsPerPage: setMessagesRowsPerPage,
      refEntries: messagesRefEntries,
      setRefEntries: setMessagesRefEntries,
      filters: messagesFilters,
      setFilters: setMessagesFilters,
      sortBy: messagesSortBy,
      setSortBy: setMessagesSortBy,
      filtersVisible: messagesFiltersVisible,
      setFiltersVisible: setMessagesFiltersVisible,
    }),
    [
      messagesSortAscending,
      messagesPage,
      messagesRowsPerPage,
      messagesRefEntries,
      messagesFilters,
      messagesSortBy,
      messagesFiltersVisible,
    ]
  );

  const transports = useMemo(
    (): TransportsViewState => ({
      sortAscending: transportsSortAscending,
      setSortAscending: setTransportsSortAscending,
      page: transportsPage,
      setPage: setTransportsPage,
      rowsPerPage: transportsRowsPerPage,
      setRowsPerPage: setTransportsRowsPerPage,
      refNames: transportsRefNames,
      setRefNames: setTransportsRefNames,
      filters: transportsFilters,
      setFilters: setTransportsFilters,
      filtersVisible: transportsFiltersVisible,
      setFiltersVisible: setTransportsFiltersVisible,
    }),
    [
      transportsSortAscending,
      transportsPage,
      transportsRowsPerPage,
      transportsRefNames,
      transportsFilters,
      transportsFiltersVisible,
    ]
  );

  const privacyGroupMessages = useMemo(
    (): PrivacyGroupMessagesViewState => ({
      sortAscending: privacyGroupMessagesSortAscending,
      setSortAscending: setPrivacyGroupMessagesSortAscending,
      page: privacyGroupMessagesPage,
      setPage: setPrivacyGroupMessagesPage,
      rowsPerPage: privacyGroupMessagesRowsPerPage,
      setRowsPerPage: setPrivacyGroupMessagesRowsPerPage,
      refEntries: privacyGroupMessagesRefEntries,
      setRefEntries: setPrivacyGroupMessagesRefEntries,
      filters: privacyGroupMessagesFilters,
      setFilters: setPrivacyGroupMessagesFilters,
      filtersVisible: privacyGroupMessagesFiltersVisible,
      setFiltersVisible: setPrivacyGroupMessagesFiltersVisible,
    }),
    [
      privacyGroupMessagesSortAscending,
      privacyGroupMessagesPage,
      privacyGroupMessagesRowsPerPage,
      privacyGroupMessagesRefEntries,
      privacyGroupMessagesFilters,
      privacyGroupMessagesFiltersVisible,
    ]
  );

  const registry = useMemo(
    (): RegistryViewState => ({
      filters: registryFilters,
      setFilters: setRegistryFilters,
      refNames: registryRefNames,
      setRefNames: setRegistryRefNames,
      sortAscending: registrySortAscending,
      setSortAscending: setRegistrySortAscending,
      page: registryPage,
      setPage: setRegistryPage,
      rowsPerPage: registryRowsPerPage,
      setRowsPerPage: setRegistryRowsPerPage,
      filtersVisible: registryFiltersVisible,
      setFiltersVisible: setRegistryFiltersVisible,
    }),
    [
      registryFilters,
      registryRefNames,
      registrySortAscending,
      registryPage,
      registryRowsPerPage,
      registryFiltersVisible,
    ]
  );

  const keys = useMemo(
    (): KeysViewState => ({
      page: keysPage,
      setPage: setKeysPage,
      rowsPerPage: keysRowsPerPage,
      setRowsPerPage: setKeysRowsPerPage,
      mode: keysMode,
      setMode: setKeysMode,
      filters: keysFilters,
      setFilters: setKeysFilters,
      sortAscending: keysSortAscending,
      setSortAscending: setKeysSortAscending,
      sortByPathFirst: keysSortByPathFirst,
      setSortByPathFirst: setKeysSortByPathFirst,
      filtersVisible: keysFiltersVisible,
      setFiltersVisible: setKeysFiltersVisible,
    }),
    [
      keysPage,
      keysRowsPerPage,
      keysMode,
      keysFilters,
      keysSortAscending,
      keysSortByPathFirst,
      keysFiltersVisible,
    ]
  );

  return (
    <ApplicationContext.Provider
      value={{
        colorMode,
        navigationVisible,
        setNavigationVisible,
        readOnly,
        setReadOnly,
        transactions,
        submissions,
        domains,
        privacyGroups,
        privacyGroupListeners,
        eventListeners,
        states,
        messages,
        transports,
        privacyGroupMessages,
        registry,
        keys,
      }}
    >
      {children}
    </ApplicationContext.Provider>
  );
};
