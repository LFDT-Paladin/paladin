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

import { CssBaseline } from "@mui/material";
import { createTheme, PaletteMode, ThemeProvider } from "@mui/material/styles";
import {
  MutationCache,
  QueryCache,
  QueryClient,
  QueryClientProvider,
} from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { constants } from "./components/config";
import { ApplicationContextProvider } from "./contexts/ApplicationContext";
import { Router } from "./Router";
import { darkThemeOptions, lightThemeOptions } from "./themes/default";

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

  return (
    <QueryClientProvider client={queryClient}>
      <ApplicationContextProvider colorMode={colorMode}>
        <ThemeProvider theme={theme}>
          <CssBaseline />
          <Router />
        </ThemeProvider>
      </ApplicationContextProvider>
    </QueryClientProvider>
  );
}

export default App;
