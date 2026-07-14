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
import { TransactionEntry } from "./views/TransactionEntry";
import { Submissions } from "./views/Submissions";
import { DomainContract } from "./views/DomainContract";
import { PrivacyGroups } from "./views/PrivacyGroups";
import { PrivacyGroupEntry } from "./views/PrivacyGroupEntry";
import { Navigation } from "./components/Navigation";
import { States } from "./views/States";
import { MessageEntry } from "./views/ReliableMessageEntry";
import { StateEntry } from "./views/StateEntry";
import { RegistryEntry } from "./views/RegistryEntry";
import { PrivacyGroupMessageEntry } from "./views/PrivacyGroupMessageEntry";
import { TransportConnections } from "./views/TransportsConnections";
import { TransportMessages } from "./views/TransportMessages";
import { PrivacyGroupListeners } from "./views/PrivacyGroupListeners";
import { PrivacyGroupListenerEntry } from "./views/PrivacyGroupListenerEntry";
import { PrivacyGroupMessages } from "./views/PrivacyGroupMessages";
import { EventListeners } from "./views/EventListeners";
import { ReceiptListeners } from "./views/ReceiptListeners";

const queryClient = new QueryClient({
  queryCache: new QueryCache({}),
  mutationCache: new MutationCache({}),
});

function App() {
  const [systemTheme, setSystemTheme] = useState(
    window.matchMedia &&
      window.matchMedia("(prefers-color-scheme: dark)").matches
      ? "dark"
      : "light"
  );

  const [storedTheme, setStoredTheme] = useState<PaletteMode>();

  useEffect(() => {
    window
      .matchMedia("(prefers-color-scheme: dark)")
      .addEventListener("change", (event) => {
        setSystemTheme(event.matches ? "dark" : "light");
      });
  }, []);

  const theme = useMemo(() => {
    const modeFromStorage = localStorage.getItem(
      constants.COLOR_MODE_STORAGE_KEY
    );
    if (modeFromStorage === null) {
      return createTheme(
        systemTheme === "dark" ? darkThemeOptions : lightThemeOptions
      );
    } else {
      return createTheme(
        modeFromStorage === "dark" ? darkThemeOptions : lightThemeOptions
      );
    }
  }, [systemTheme, storedTheme]);

  const colorMode = useMemo(
    () => ({
      toggleColorMode: () => {
        const currentMode =
          localStorage.getItem(constants.COLOR_MODE_STORAGE_KEY) ?? systemTheme;
        const newMode = currentMode === "light" ? "dark" : "light";
        localStorage.setItem(constants.COLOR_MODE_STORAGE_KEY, newMode);
        setStoredTheme(newMode);
      },
    }),
    [systemTheme]
  );

  const basePath = getBasePath();
  const lessThanLarge = useMediaQuery(theme.breakpoints.down("lg"));

  return (
    <QueryClientProvider client={queryClient}>
      <ApplicationContextProvider colorMode={colorMode}>
        <ThemeProvider theme={theme}>
          <CssBaseline />
          <BrowserRouter
            basename={basePath}
            future={{ v7_relativeSplatPath: true, v7_startTransition: true }}
          >
            {lessThanLarge && <Header />}
            <Box sx={{ display: "flex" }}>
              <Navigation />
              <Box sx={{ flexGrow: 1, maxWidth: "100vw", minWidth: 0 }}>
                <Routes>
                  <Route path={AppRoutes.Transactions} element={<Transactions />} />
                  <Route path={AppRoutes.Submissions} element={<Submissions />} />
                  <Route path={AppRoutes.Transaction} element={<TransactionEntry />} />
                  <Route path={AppRoutes.Keys} element={<Keys />} />
                  <Route path={AppRoutes.Registies} element={<Registries />} />
                  <Route path={AppRoutes.Domains} element={<Domains />} />
                  <Route path={AppRoutes.DomainContract} element={<DomainContract />} />
                  <Route path={AppRoutes.PrivacyGroups} element={<PrivacyGroups />} />
                  <Route path={AppRoutes.PrivacyGroupMessages} element={<PrivacyGroupMessages />} />
                  <Route path={AppRoutes.PrivacyGroupListeners} element={<PrivacyGroupListeners />} />
                  <Route path={AppRoutes.PrivacyGroup} element={<PrivacyGroupEntry />} />
                  <Route path={AppRoutes.PrivacyGroupMessageEntry} element={<PrivacyGroupMessageEntry />} />
                  <Route path={AppRoutes.PrivacyGroupListenerEntry} element={<PrivacyGroupListenerEntry />} />
                  <Route path={AppRoutes.States} element={<States />} />
                  <Route path={AppRoutes.State} element={<StateEntry />} />
                  <Route path={AppRoutes.RegistryEntry} element={<RegistryEntry />} />
                  <Route path={AppRoutes.ReliableMessage} element={<MessageEntry />} />
                  <Route path={AppRoutes.TransportConnections} element={<TransportConnections />} />
                  <Route path={AppRoutes.TransportMessages} element={<TransportMessages />} />
                  <Route path={AppRoutes.EventListeners} element={<EventListeners />} />
                  <Route path={AppRoutes.ReceiptListeners} element={<ReceiptListeners />} />
                  <Route path="*" element={<Navigate to={AppRoutes.Transactions} replace />} />
                </Routes>
              </Box>
            </Box>
          </BrowserRouter>
        </ThemeProvider>
      </ApplicationContextProvider>
    </QueryClientProvider>
  );
}

export default App;
