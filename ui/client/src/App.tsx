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

import { Box, CssBaseline, useMediaQuery } from "@mui/material";
import { createTheme, PaletteMode, ThemeProvider } from "@mui/material/styles";
import {
  MutationCache,
  QueryCache,
  QueryClient,
  QueryClientProvider,
} from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { constants } from "./components/config";
import { Header } from "./components/Header";
import { ApplicationContextProvider } from "./contexts/ApplicationContext";
import { AppRoutes } from "./routes";
import { darkThemeOptions, lightThemeOptions } from "./themes/default";
import { getBasePath } from "./utils";
import { Domains } from "./views/Domains";
import { Keys } from "./views/Keys";
import { Registries } from "./views/Registries";
import { Transactions } from "./views/Transactions";
import { TransactionDetails } from "./views/TransactionDetails";
import { IPaladinTransactionPagingReference, ITransactionPagingReference } from "./interfaces";
import { Submissions } from "./views/Submissions";
import { DomainContract } from "./views/DomainContract";
import { PrivacyGroups } from "./views/PrivacyGroups";
import { PrivacyGroup } from "./views/PrivacyGroup";
import { Navigation } from "./components/Navigation";
import { States } from "./views/States";
import { Messages } from "./views/Messages";
import { Message } from "./views/Message";
import { State } from "./views/State";

const queryClient = new QueryClient({
  queryCache: new QueryCache({}),
  mutationCache: new MutationCache({}),
});

function App() {

  const [txRefEntries, setTxRefEntries] = useState<ITransactionPagingReference[]>([]);
  const [txPage, txSetPage] = useState(0);
  const [txRowsPerPage, setTxRowsPerPage] = useState(10);
  const [txFromBlock, setTxFromBlock] = useState<number>();
  const [submissionsSection, setSubmissionsSection] = useState<'pending' | 'failed'>('pending');
  const [domainSortAscending, setDomainSortAscending] = useState(false);
  const [domainsPage, txSetDomainsPage] = useState(0);
  const [domainsRowsPerPage, SetDomainsRowsPerPage] = useState(10);
  const [domainsRefTimestamps, setDomainsRefTimestamps] = useState<string[]>([]);
  const [submissionsRefEntries, setSubmissionsSetRefEntries] = useState<IPaladinTransactionPagingReference[]>([]);
  const [submissionsPage, setSubmissionsPage] = useState(0);
  const [submissionsRowsPerPage, setSubmissionsRowsPerPage] = useState(10);
  const [domainsSelectedDomain, setDomainsSelectedDomain] = useState<string>();
  const [privacyGroupsPage, setPrivacyGroupsPage] = useState(0);
  const [privacyGroupsRowsPerPage, setPrivacyGroupsRowsPerPage] = useState(10);
  const [privacyGroupsRefTimestamps, sePrivacyGroupsRefTimestamps] = useState<string[]>([]);
  const [privacyGroupsSortAscending, setPrivacyGroupsSortAscending] = useState(false);
  const [navigationVisible, setNavigationVisible] = useState(false);
  const [statesSelectedDomain, setStatesSelectedDomain] = useState<string>();
  const [statesSelectedSchemaId, setStatesSelectedSchemaId] = useState<string>();
  const [statePage, setStatePage] = useState(0);
  const [stateRowsPerPage, setStateRowsPerPage] = useState(10);
  const [stateRefTimestamps, setStateRefTimestamps] = useState<string[]>([]);
  const [stateSortAscending, setStateSortAscending] = useState(false);
  const [messagesPage, setMessagesPage] = useState(0);
  const [messagesRowsPerPage, setMessagesRowsPerPage] = useState(10);
  const [messagesRefTimestamps, setMessagesRefTimestamps] = useState<string[]>([]);
  const [messagesSortAscending, setMessagesSortAscending] = useState(false);

  const [systemTheme, setSystemTheme] = useState(
    window.matchMedia &&
      window.matchMedia('(prefers-color-scheme: dark)').matches
      ? 'dark'
      : 'light'
  );

  const [storedTheme, setStoredTheme] = useState<PaletteMode>();

  useEffect(() => {
    window
      .matchMedia('(prefers-color-scheme: dark)')
      .addEventListener('change', (event) => {
        setSystemTheme(event.matches ? 'dark' : 'light');
      });
  }, []);


  const theme = useMemo(() => {
    const modeFromStorage = localStorage.getItem(constants.COLOR_MODE_STORAGE_KEY);
    if (modeFromStorage === null) {
      // If color mode not previously set
      return createTheme(
        systemTheme === 'dark' ? darkThemeOptions : lightThemeOptions
      );
    } else {
      // Create color mode based on local storage
      return createTheme(
        modeFromStorage === 'dark' ? darkThemeOptions : lightThemeOptions
      );
    }
  }, [systemTheme, storedTheme]);

  const colorMode = useMemo(
    () => ({
      toggleColorMode: () => {
        const currentMode =
          localStorage.getItem(constants.COLOR_MODE_STORAGE_KEY) ?? systemTheme;
        const newMode = currentMode === 'light' ? 'dark' : 'light';
        localStorage.setItem(constants.COLOR_MODE_STORAGE_KEY, newMode);
        setStoredTheme(newMode);
      },
    }),
    []
  );

  const basePath = getBasePath();

  const lessThanLarge = useMediaQuery(theme.breakpoints.down('lg'));

  return (
    <>
      <QueryClientProvider client={queryClient}>
        <ApplicationContextProvider colorMode={colorMode}>
          <ThemeProvider theme={theme}>
            <CssBaseline />
            <BrowserRouter basename={basePath}
              future={{ v7_relativeSplatPath: true, v7_startTransition: true }}
            >
              {lessThanLarge &&
                <Header
                  navigationVisible={navigationVisible}
                  setNavigationVisible={setNavigationVisible}
                />}
              <Box sx={{ display: 'flex' }}>
                <Navigation
                  navigationVisible={navigationVisible}
                  setNavigationVisible={setNavigationVisible}
                />

                <Box sx={{ flexGrow: 1, maxWidth: '100vw', minWidth: 0 }}>
                  <Routes>
                    <Route path={AppRoutes.Transactions} element={<Transactions
                      refEntries={txRefEntries}
                      setRefEntries={setTxRefEntries}
                      page={txPage}
                      setPage={txSetPage}
                      rowsPerPage={txRowsPerPage}
                      setRowsPerPage={setTxRowsPerPage}
                      fromBlock={txFromBlock}
                      setFromBlock={setTxFromBlock}
                    />} />
                    <Route path={AppRoutes.Submissions} element={<Submissions
                      section={submissionsSection}
                      setSection={setSubmissionsSection}
                      page={submissionsPage}
                      setPage={setSubmissionsPage}
                      rowsPerPage={submissionsRowsPerPage}
                      setRowsPerPage={setSubmissionsRowsPerPage}
                      refEntries={submissionsRefEntries}
                      setRefEntries={setSubmissionsSetRefEntries}
                    />} />
                    <Route path={AppRoutes.Transaction} element={<TransactionDetails />} />
                    <Route path={AppRoutes.Keys} element={<Keys />} />
                    <Route path={AppRoutes.Registry} element={<Registries />} />
                    <Route path={AppRoutes.Domains} element={<Domains
                      sortAscending={domainSortAscending}
                      setSortAscending={setDomainSortAscending}
                      page={domainsPage}
                      setPage={txSetDomainsPage}
                      rowsPerPage={domainsRowsPerPage}
                      setRowsPerPage={SetDomainsRowsPerPage}
                      refTimestamps={domainsRefTimestamps}
                      setRefTimestamps={setDomainsRefTimestamps}
                      selectedDomain={domainsSelectedDomain}
                      setSelectedDomain={setDomainsSelectedDomain}
                    />} />
                    <Route path={AppRoutes.DomainContract} element={<DomainContract />} />
                    <Route path={AppRoutes.PrivactGroups} element={<PrivacyGroups
                      sortAscending={privacyGroupsSortAscending}
                      setSortAscending={setPrivacyGroupsSortAscending}
                      refTimestamps={privacyGroupsRefTimestamps}
                      setRefTimestamps={sePrivacyGroupsRefTimestamps}
                      page={privacyGroupsPage}
                      setPage={setPrivacyGroupsPage}
                      rowsPerPage={privacyGroupsRowsPerPage}
                      setRowsPerPage={setPrivacyGroupsRowsPerPage}
                    />} />
                    <Route path={AppRoutes.PrivacyGroup} element={<PrivacyGroup />} />
                    <Route path={AppRoutes.States} element={<States
                      selectedDomain={statesSelectedDomain}
                      setSelectedDomain={setStatesSelectedDomain}
                      selectedSchemaId={statesSelectedSchemaId}
                      setSelectedSchemaId={setStatesSelectedSchemaId}
                      sortAscending={stateSortAscending}
                      setSortAscending={setStateSortAscending}
                      refTimestamps={stateRefTimestamps}
                      setRefTimestamps={setStateRefTimestamps}
                      page={statePage}
                      setPage={setStatePage}
                      rowsPerPage={stateRowsPerPage}
                      setRowsPerPage={setStateRowsPerPage}
                    />} />
                    <Route path={AppRoutes.State} element={<State />} />
                    <Route path={AppRoutes.Messages} element={<Messages
                      sortAscending={messagesSortAscending}
                      setSortAscending={setMessagesSortAscending}
                      page={messagesPage}
                      setPage={setMessagesPage}
                      rowsPerPage={messagesRowsPerPage}
                      setRowsPerPage={setMessagesRowsPerPage}
                      refTimestamps={messagesRefTimestamps}
                      setRefTimestamps={setMessagesRefTimestamps}
                    />} />
                    <Route path={AppRoutes.Message} element={<Message />} />
                    <Route path="*" element={<Navigate to={AppRoutes.Transactions} replace />} />
                  </Routes>
                </Box>
              </Box>
            </BrowserRouter>
          </ThemeProvider>
        </ApplicationContextProvider>
      </QueryClientProvider>
    </>
  );
}

export default App;
