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

import { Box, Paper, Tab, Tabs } from "@mui/material";
import { IPaladinTransaction } from "../interfaces";
import { useTranslation } from "react-i18next";
import { getShortId } from "../utils";
import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { PaladinTransactionsDetails } from "./TransactionDetails";

type Props = {
  paladinTransactions: IPaladinTransaction[]
}

export const PaladinTransactionSection: React.FC<Props> = ({ paladinTransactions }) => {

  const { hash, id } = useParams();
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [selectedPaladinTransactionId, setSelectedPaladinTransactionId] = useState<string>(
    id ?? paladinTransactions[0].id);

  return (
    <>
      <Tabs
        value={selectedPaladinTransactionId}
        TabIndicatorProps={{ style: { display: 'none' } }}
        onChange={(_event, value) => {
          navigate(`/ui/transactions/${hash}/${value}`, { replace: true });
          setSelectedPaladinTransactionId(value);
        }}
      >
        {paladinTransactions.map(paladinTransaction =>
          <Tab key={paladinTransaction.id} value={paladinTransaction.id}
            sx={{
              backgroundColor:
                selectedPaladinTransactionId === paladinTransaction.id ?
                  theme => theme.palette.background.paper : 'inherit',
              borderTopLeftRadius: '4px',
              borderTopRightRadius: '4px'
            }}
            label={
              <Box>
                <span style={{ fontWeight: 600, marginRight: '6px' }}>{t(paladinTransaction.domain)}</span>
                {getShortId(paladinTransaction.id)}
              </Box>
            } />
        )}
      </Tabs>
      <Paper elevation={0} sx={{
        borderTopLeftRadius: 0
      }}>
        <PaladinTransactionsDetails
          paladinTransaction={paladinTransactions.find(tx => tx.id === selectedPaladinTransactionId)}
        />
      </Paper>
    </>);
}