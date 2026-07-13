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

import { Alert, Box, Button, Collapse, Fade, TablePagination, ToggleButton, ToggleButtonGroup, Typography } from "@mui/material";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { fetchIndexedTransactions } from "../queries/transactions";
import { EnrichedTransaction } from "../components/EnrichedTransaction";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import SearchIcon from '@mui/icons-material/Search';
import { TransactionLookupDialog } from "../dialogs/TransactionLookup";
import { useApplicationContext } from "../contexts/ApplicationContext";
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import { Filters } from "../components/Filters";
import { FiltersButton } from "../components/FiltersButton";
import { pagedTableCount, useResetPaginationOnChange } from "../hooks/pagination";

export const Transactions: React.FC = () => {
  const { transactions } = useApplicationContext();
  const {
    refEntries,
    setRefEntries,
    page,
    setPage,
    rowsPerPage,
    setRowsPerPage,
    showTxsWithReceipt,
    setShowTxsWithReceipt,
    filters,
    setFilters,
    filtersVisible,
    setFiltersVisible,
  } = transactions;
  const [lookupTransactionDialogOpen, setLookupTransactionDialogOpen] = useState(false);
  const { t } = useTranslation();

  const { data, error, isPlaceholderData, isFetching } = useQuery({
    queryKey: ['transactions', refEntries, rowsPerPage, showTxsWithReceipt, filters, page],
    queryFn: () => fetchIndexedTransactions(rowsPerPage, showTxsWithReceipt, filters, refEntries[refEntries.length - 1]),
    placeholderData: keepPreviousData
  });

  const enrichedTransactions = data?.items;
  const hasMore = data?.hasMore ?? false;

  const count = pagedTableCount(data, hasMore, page, rowsPerPage);

  useResetPaginationOnChange(() => {
    setPage(0);
    setRefEntries([]);
  }, showTxsWithReceipt);

  useResetPaginationOnChange(() => {
    setPage(0);
    setRefEntries([]);
  }, filters);

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
      if (enrichedTransactions !== undefined && !isPlaceholderData && enrichedTransactions.length > 0) {
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
            maxWidth: "1500px",
            marginLeft: "auto",
            marginRight: "auto",
          }}
        >
          <Box sx={{ display: 'flex', alignItems: 'center', gap: '20px', marginBottom: '20px', flexWrap: 'wrap' }}>
            <Typography variant="h5">
              {t("transactions")}
            </Typography>
            <ToggleButtonGroup size="small" sx={{ height: '30px' }} exclusive onChange={
              (_event, value) => {
                if (value !== null) {
                  setShowTxsWithReceipt(value === 'withReceipt')
                }
              }
            } value={showTxsWithReceipt ? 'withReceipt' : 'all'}>
              <ToggleButton color="primary" value="withReceipt" sx={{ width: '120px' }}>{t('paladinOnly')}</ToggleButton>
              <ToggleButton color="primary" value="all" sx={{ width: '120px' }}>{t('all')}</ToggleButton>
            </ToggleButtonGroup>
            <Box sx={{ flexGrow: 1, display: 'flex', justifyContent: 'right', gap: '10px' }}>
              <Button
                sx={{ borderRadius: '20px', minWidth: '120px' }}
                size="small"
                variant="outlined"
                startIcon={<SearchIcon />}
                onClick={() => setLookupTransactionDialogOpen(true)}
              >
                {t('lookup')}
              </Button>
              <FiltersButton
                filtersVisible={filtersVisible}
                setFiltersVisible={setFiltersVisible}
              />
            </Box>
          </Box>
          <Collapse in={filtersVisible}>
            <Box sx={{ marginBottom: '20px' }}>
              <Filters
                filterFields={[
                  {
                    label: t('transactionHash'),
                    name: 'hash',
                    type: 'string',
                    isHexValue: true
                  },
                  {
                    label: t('block'),
                    name: 'blockNumber',
                    type: 'number'
                  },
                  {
                    label: t('transactionIndex'),
                    name: 'transactionIndex',
                    type: 'number'
                  },
                  {
                    label: t('nonce'),
                    name: 'nonce',
                    type: 'number'
                  },
                  {
                    label: t('from'),
                    name: 'from',
                    type: 'string',
                    isHexValue: true
                  },
                  {
                    label: t('to'),
                    name: 'to',
                    type: 'string',
                    isHexValue: true
                  },
                  {
                    label: t('status'),
                    name: 'result',
                    type: 'enum',
                    enum: ['success', 'failed']
                  }
                ]}
                filters={filters}
                setFilters={setFilters}
              />
            </Box>
          </Collapse>
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
          {enrichedTransactions !== undefined && enrichedTransactions.length > 0 &&
            <TablePagination
              slotProps={{
                actions: {
                  lastButton: {
                    disabled: true
                  },
                  nextButton: {
                    disabled: !hasMore || isFetching || isPlaceholderData
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
            />}
          {enrichedTransactions !== undefined && enrichedTransactions.length === 0 &&
            <Box sx={{ marginTop: '20px', textAlign: 'center', color: theme => theme.palette.text.secondary }}>
              <InfoOutlinedIcon sx={{ fontSize: '50px' }} />
              <Typography>{t('transactionsEmptyState')}</Typography>
            </Box>
          }
        </Box>
      </Fade>
      <TransactionLookupDialog
        dialogOpen={lookupTransactionDialogOpen}
        setDialogOpen={setLookupTransactionDialogOpen}
        label={t('blockchainTransactionHashOrPaladinTransactionId')}
      />
    </>
  );
}
