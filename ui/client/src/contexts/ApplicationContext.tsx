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

import { useQuery } from "@tanstack/react-query";
import { createContext, Dispatch, SetStateAction, useEffect, useState } from "react";
import { ErrorDialog } from "../dialogs/Error";
import { fetchLatestBlockWithTxs } from "../queries/blocks";
import { constants } from "../components/config";

interface IApplicationContext {
  colorMode: {
    toggleColorMode: () => void;
  };
  lastBlockWithTransactions: number;
  autoRefreshEnabled: boolean;
  setAutoRefreshEnabled: Dispatch<SetStateAction<boolean>>;
  refreshRequired: boolean;
  refresh: () => void;
}

export const ApplicationContext = createContext({} as IApplicationContext);

interface Props {
  colorMode: {
    toggleColorMode: () => void;
  };
  children: JSX.Element;
}

export const ApplicationContextProvider = ({ children, colorMode }: Props) => {

  const [autoRefreshEnabled, setAutoRefreshEnabled] = useState(false);
  const [lastBlockWithTransactions, setLastBlockWithTransactions] = useState(-1);
  const [refreshRequired, setRefreshRequired] = useState(false);

  const { data: actualLastBlockWithTransactions, error } = useQuery({
    queryKey: ["lastBlockWithTransactions"],
    queryFn: () =>
      fetchLatestBlockWithTxs().then((res) => {
        if (res.length > 0) {
          return res[0].blockNumber;
        }
        return 0;
      }),
    refetchInterval: constants.UPDATE_FREQUENCY_MILLISECONDS,
    retry: false
  });

  useEffect(() => {


    if(actualLastBlockWithTransactions !== undefined
      && actualLastBlockWithTransactions > lastBlockWithTransactions) {
        
        if(autoRefreshEnabled || lastBlockWithTransactions === -1) {
          setLastBlockWithTransactions(actualLastBlockWithTransactions);
        } else {
          setRefreshRequired(true);
        }

    }
  }, [actualLastBlockWithTransactions, lastBlockWithTransactions, setLastBlockWithTransactions]);

  const refresh = () => {
    if(actualLastBlockWithTransactions !== undefined) {
      setLastBlockWithTransactions(actualLastBlockWithTransactions);
    }
    setRefreshRequired(false);
  };

  return (
    <ApplicationContext.Provider
      value={{
        lastBlockWithTransactions: lastBlockWithTransactions, colorMode,
        autoRefreshEnabled, setAutoRefreshEnabled, refreshRequired, refresh
      }}
    >
      {children}
      <ErrorDialog dialogOpen={!!error} message={error?.message ?? ""} />
    </ApplicationContext.Provider>
  );
};
