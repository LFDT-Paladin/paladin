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

import { Alert, Box, Button, Fade, Grid2, TablePagination, Typography } from "@mui/material";
import { useQuery } from "@tanstack/react-query";
import { fetchIndexedTransactions } from "../queries/transactions";
import { EnrichedTransaction } from "../components/EnrichedTransaction";
import { Dispatch, SetStateAction, useContext, useEffect, useState } from "react";
import { ITransactionPagingReference } from "../interfaces";
import { useTranslation } from "react-i18next";
import SearchIcon from '@mui/icons-material/Search';
import { TransactionLookupDialog } from "../dialogs/TransactionLookup";
import { ApplicationContext } from "../contexts/ApplicationContext";
import ViewArrayOutlinedIcon from '@mui/icons-material/ViewArrayOutlined';
import { FromBlockDialog } from "../dialogs/FromBlock";

type Props = {
  refEntries: ITransactionPagingReference[]
  setRefEntries: Dispatch<SetStateAction<ITransactionPagingReference[]>>
  page: number
  setPage: Dispatch<SetStateAction<number>>
  fromBlock: number | undefined
  setFromBlock: Dispatch<SetStateAction<number | undefined>>
  rowsPerPage: number
  setRowsPerPage: Dispatch<SetStateAction<number>>
};

export const Transactions: React.FC<Props> = ({
  refEntries,
  setRefEntries,
  page,
  setPage,
  rowsPerPage,
  setRowsPerPage,
  fromBlock,
  setFromBlock
}) => {

  const { lastBlockWithTransactions } = useContext(ApplicationContext);
  const [lookupTransactionDialogOpen, setLookupTransactionDialogOpen] = useState(false);
  const [fromBlockDialogOpen, setFromBlockDialogOpen] = useState(false);
  const [count, setCount] = useState(-1);
  const { t } = useTranslation();

  const { data: enrichedTransactions, error } = useQuery({
    queryKey: ['transactions', refEntries, rowsPerPage, page, lastBlockWithTransactions, fromBlock],
    queryFn: () => fetchIndexedTransactions(rowsPerPage, fromBlock, refEntries[refEntries.length - 1])
  });

  useEffect(() => {
    if (enrichedTransactions !== undefined && count === -1) {
      if (enrichedTransactions.length < rowsPerPage) {
        setCount(rowsPerPage * page + enrichedTransactions.length);
      }
    }
  }, [enrichedTransactions, rowsPerPage, page]);

  if (error) {
    return <Alert sx={{ margin: '30px' }} severity="error" variant="filled">{error.message}</Alert>
  }

  const handleChangePage = (
    _event: React.MouseEvent<HTMLButtonElement> | null,
    newPage: number
  ) => {
    if (newPage === 0) {
      setRefEntries([]);
    } else if (newPage > page) {
      if (enrichedTransactions !== undefined) {
        const refEntriesCopy = [...refEntries];
        refEntriesCopy.push(enrichedTransactions[enrichedTransactions.length - 1]);
        setRefEntries(refEntriesCopy);
      }
    } else {
      const refEntriesCopy = [...refEntries];
      refEntriesCopy.pop();
      setRefEntries(refEntriesCopy);
    }
    setPage(newPage);
  };

  const handleChangeRowsPerPage = (
    event: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>
  ) => {
    const value = parseInt(event.target.value, 10);
    setRowsPerPage(value);
    setRefEntries([]);
    setPage(0);
  };

  return (
    <>
      <Fade timeout={600} in={true}>
        <Box
          sx={{
            padding: "20px",
            maxWidth: "1300px",
            marginLeft: "auto",
            marginRight: "auto",
          }}
        >
          <Box sx={{ marginBottom: '20px' }}>
            <Grid2 container alignItems="center" spacing={2}>
              <Grid2 sx={{ display: { xs: 'none', sm: 'none', md: 'block' } }} size={{ md: 4 }} />
              <Grid2 size={{ xs: 12, md: 4 }}>
                <Typography align="center" variant="h5">
                  {t("transactions")}
                </Typography>
              </Grid2>
              <Grid2 size={{ xs: 12, md: 4 }} container justifyContent="right">
                <Grid2>
                  <Button
                    sx={{ borderRadius: '20px', minWidth: '180px' }}
                    size="large"
                    variant="outlined"
                    startIcon={<SearchIcon />}
                    onClick={() => setLookupTransactionDialogOpen(true)}
                  >
                    {t('lookup')}
                  </Button>
                </Grid2>
                <Grid2>
                  <Button
                    color={fromBlock === undefined ? 'secondary' : 'warning'}
                    size="large"
                    variant="outlined"
                    startIcon={<ViewArrayOutlinedIcon />}
                    sx={{ borderRadius: '20px', minWidth: '180px' }}
                    onClick={() => setFromBlockDialogOpen(true)}
                  >
                    {t(fromBlock === undefined ? 'Latest block' : 'fromBlockN', { n: fromBlock?.toLocaleString() })}
                  </Button>
                </Grid2>
              </Grid2>
            </Grid2>
          </Box>
          <Box sx={{
            display: 'flex',
            flexDirection: 'column',
            gap: '20px'
          }}>
            {enrichedTransactions?.map(enrichedTransaction =>
              <EnrichedTransaction key={`${enrichedTransaction.block}:${enrichedTransaction.hash}`}
                enrichedTransaction={enrichedTransaction}
              />
            )}
          </Box>
          <TablePagination
            slotProps={{
              actions: {
                lastButton: {
                  disabled: true
                }
              }
            }}

            component="div"
            showFirstButton
            showLastButton
            count={count}
            page={page}
            onPageChange={handleChangePage}
            rowsPerPage={rowsPerPage}
            onRowsPerPageChange={handleChangeRowsPerPage}
          />
        </Box>
      </Fade>
      <TransactionLookupDialog
        dialogOpen={lookupTransactionDialogOpen}
        setDialogOpen={setLookupTransactionDialogOpen}
        label={t('blockchainTransactionHashOrPaladinTransactionId')}
      />
      <FromBlockDialog
        dialogOpen={fromBlockDialogOpen}
        setDialogOpen={setFromBlockDialogOpen}
        fromBlock={fromBlock}
        setFromBlock={setFromBlock}
        setPage={setPage}
        setRefEntries={setRefEntries}
      />
    </>
  );
}
