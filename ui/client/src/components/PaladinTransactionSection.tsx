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
import { ITransactionReceipt } from "../interfaces";
import { useTranslation } from "react-i18next";
import { getShortId, isValidUUID } from "../utils";
import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { PaladinTransactionsDetails } from "./TransactionDetails";

type Props = {
  receipts: ITransactionReceipt[]
}

export const PaladinTransactionSection: React.FC<Props> = ({ receipts }) => {

  const { hashOrId } = useParams();
  const { t } = useTranslation();
  const navigate = useNavigate();

  const [selectedReceiptId, setSelectedReceiptId] = useState<string>(
    (hashOrId !== undefined && isValidUUID(hashOrId)) ? hashOrId : receipts[0].id);

  return (
    <>
      <Tabs
        value={selectedReceiptId}
        TabIndicatorProps={{ style: { display: 'none' } }}
        onChange={(_event, value) => {
          navigate(`/ui/transactions/${value}`, { replace: true });
          setSelectedReceiptId(value);
        }}
      >
        {receipts.map(receipt =>
          <Tab key={receipt.id} value={receipt.id}
            sx={{
              backgroundColor:
                selectedReceiptId === receipt.id ?
                  theme => theme.palette.background.paper : 'inherit',
              borderTopLeftRadius: '4px',
              borderTopRightRadius: '4px'
            }}
            label={
              <Box>
                <span style={{ fontWeight: 600, marginRight: '6px' }}>{t(receipt.domain ?? 'public')}</span>
                {getShortId(receipt.id)}
              </Box>
            } />
        )}
      </Tabs>
      <Paper elevation={0} sx={{
        borderTopLeftRadius: 0
      }}>
        <PaladinTransactionsDetails
          receipt={receipts.find(receipt => receipt.id === selectedReceiptId)!}
        />
      </Paper>
    </>);
}