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

import { Box, Paper, Table, TableBody, TableCell, TableRow, Typography } from "@mui/material";
import { ITransaction } from "../interfaces";
import { useTranslation } from "react-i18next";
import { Hash } from "./Hash";
import { Captions } from "lucide-react";
import { TransactionStatus } from "./TransactionStatus";
import { EllapsedTime } from "./EllapsedTime";

type Props = {
  transaction: ITransaction
}

export const TransactionOverview: React.FC<Props> = ({ transaction }) => {

  const { t } = useTranslation();

  return (
    <Paper elevation={0}
    >
      <Table>
        <TableBody
          sx={{
            "& .MuiTableRow-root:last-child td, & .MuiTableRow-root:last-child th": {
              borderBottom: "none",
            },
          }}>
          <TableRow>
            <TableCell width={'140px'}>
              <Typography variant="body2" color="textSecondary">{t('hash')}</Typography>
            </TableCell>
            <TableCell sx={{ paddingTop: 0, paddingBottom: 0 }}>
              <Box sx={{ maxWidth: '140px' }}>
                <Hash Icon={<Captions size="18px" />} title={t('hash')} hash={transaction.hash} hideTitle />
              </Box>
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell>
              <Typography variant="body2" color="textSecondary">{t('block')}</Typography>
            </TableCell>
            <TableCell sx={{ paddingTop: 0, paddingBottom: 0}}>
              <Typography variant="body2" color="textPrimary">{transaction.blockNumber.toLocaleString()}</Typography>
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell>
              <Typography variant="body2" color="textSecondary">{t('transactionIndex')}</Typography>
            </TableCell>
            <TableCell sx={{ paddingTop: 0, paddingBottom: 0}}>
              <Typography variant="body2" color="textPrimary">{transaction.transactionIndex.toLocaleString()}</Typography>
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell>
              <Typography variant="body2" color="textSecondary">{t('nonce')}</Typography>
            </TableCell>
            <TableCell sx={{ paddingTop: 0, paddingBottom: 0}}>
              <Typography variant="body2" color="textPrimary">{transaction.nonce.toLocaleString()}</Typography>
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell>
              <Typography variant="body2" color="textSecondary">{t('from')}</Typography>
            </TableCell>
            <TableCell sx={{ paddingTop: 0, paddingBottom: 0}}>
              <Box sx={{ maxWidth: '140px' }}>
                <Hash Icon={<Captions size="18px" />} title={t('from')} hash={transaction.from} hideTitle />
              </Box>
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell>
              <Typography variant="body2" color="textSecondary">{t('to')}</Typography>
            </TableCell>
            <TableCell sx={{ paddingTop: 0, paddingBottom: 0}}>
              <Box sx={{ width: '140px' }}>
                {transaction.to ?
                  <Hash Icon={<Captions size="18px" />} title={t('to')} hash={transaction.to} hideTitle />
                  :
                  <Typography variant="body2" color="textPrimary">--</Typography>
                }
              </Box>
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell>
              <Typography variant="body2" color="textSecondary">{t('contract')}</Typography>
            </TableCell>
            <TableCell>
              <Box sx={{ width: '140px' }}>
                {transaction.contractAddress ?
                  <Hash Icon={<Captions size="18px" />} title={t('to')} hash={transaction.contractAddress} hideTitle />
                  :
                  <Typography variant="body2" color="textPrimary">--</Typography>
                }
              </Box>
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell>
              <Typography variant="body2" color="textSecondary">{t('status')}</Typography>
            </TableCell>
            <TableCell sx={{ paddingTop: 0, paddingBottom: 0 }}>

              <TransactionStatus transaction={transaction} />
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell>
              <Typography variant="body2" color="textSecondary">{t('time')}</Typography>
            </TableCell>
            <TableCell sx={{ paddingTop: 0, paddingBottom: 0 }}>
              <EllapsedTime icon={<></>} timestamp={transaction.block.timestamp} />
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </Paper>
  );

};