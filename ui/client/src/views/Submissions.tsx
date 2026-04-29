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

import { Alert, Box, Button, Fade, Grid2, TablePagination, ToggleButton, ToggleButtonGroup, Typography, useTheme } from "@mui/material";
import { useQuery } from "@tanstack/react-query";
import { Dispatch, SetStateAction, useContext, useEffect, useState } from "react";
import { PaladinTransaction } from "../components/PaladinTransaction";
import { ApplicationContext } from "../contexts/ApplicationContext";
import { fetchSubmissions } from "../queries/transactions";
import { getAltModeScrollBarStyle } from "../themes/default";
import { IFilter, IPaladinTransactionPagingReference } from "../interfaces";
import { useTranslation } from "react-i18next";
import { Filters } from "../components/Filters";
import { constants } from "../components/config";
import SearchIcon from '@mui/icons-material/Search';
import { TransactionLookupDialog } from "../dialogs/TransactionLookup";

type Props = {
  section: 'pending' | 'failed'
  setSection: Dispatch<SetStateAction<'pending' | 'failed'>>
  page: number
  setPage: Dispatch<SetStateAction<number>>
  rowsPerPage: number
  setRowsPerPage: Dispatch<SetStateAction<number>>
  refEntries: IPaladinTransactionPagingReference[]
  setRefEntries: Dispatch<SetStateAction<IPaladinTransactionPagingReference[]>>
};

export const Submissions: React.FC<Props> = ({
  section,
  setSection,
  page,
  setPage,
  rowsPerPage,
  setRowsPerPage,
  refEntries,
  setRefEntries
}) => {

  const getFiltersFromStorage = () => {
    const value = window.localStorage.getItem(constants.SUBMISSIONS_FILTERS_KEY);
    if (value !== null) {
      try {
        return JSON.parse(value);
      } catch (_err) { }
    }
    return [];
  };

  const [lookupTransactionDialogOpen, setLookupTransactionDialogOpen] = useState(false);
  const { lastBlockWithTransactions } = useContext(ApplicationContext);
  const [filters, setFilters] = useState<IFilter[]>(getFiltersFromStorage());
  const [count, setCount] = useState(-1);
  const theme = useTheme();
  const { t } = useTranslation();

  const { data: transactions, error } = useQuery({
    queryKey: ['submissions', section, lastBlockWithTransactions, filters, refEntries, rowsPerPage, page],
    queryFn: () => fetchSubmissions(section, filters, refEntries[refEntries.length - 1])
  });

  useEffect(() => {
    window.localStorage.setItem(constants.SUBMISSIONS_FILTERS_KEY, JSON.stringify(filters));
  }, [filters]);

  useEffect(() => {
    if (transactions !== undefined && count === -1) {
      if (transactions.length < rowsPerPage) {
        setCount(rowsPerPage * page + transactions.length);
      }
    }
  }, [transactions, rowsPerPage, page]);

  const handleChangePage = (
    _event: React.MouseEvent<HTMLButtonElement> | null,
    newPage: number
  ) => {
    if (newPage === 0) {
      setRefEntries([]);
    } else if (newPage > page) {
      if (transactions !== undefined) {
        const refEntriesCopy = [...refEntries];
        refEntriesCopy.push(transactions[transactions.length - 1]);
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

  if (error) {
    return <Alert sx={{ margin: '30px' }} severity="error" variant="filled">{error.message}</Alert>
  }

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
          <Grid2 container alignItems="center" spacing={2}>
            <Grid2 sx={{ display: { xs: 'none', sm: 'none', md: 'block' } }} size={{ md: 4 }} />
            <Grid2 size={{ xs: 12, md: 4 }}>
              <Typography align="center" variant="h5">
                {t("submissions")}
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
            </Grid2>
          </Grid2>
          <Box sx={{ marginTop: '15px', marginBottom: '15px', textAlign: 'center' }}>
            <ToggleButtonGroup exclusive onChange={(_event, value) => {
              if (value !== null) {
                setSection(value)
              }
            }
            } value={section}>
              <ToggleButton color="primary" value="pending" sx={{ width: '130px', height: '45px' }}>{t('pending')}</ToggleButton>
              <ToggleButton color="primary" value="failed" sx={{ width: '130px', height: '45px' }}>{t('failed')}</ToggleButton>
            </ToggleButtonGroup>
          </Box>
          <Box sx={{ marginBottom: '20px' }}>
            <Filters
              filterFields={[
                {
                  label: t('from'),
                  name: 'from',
                  type: 'string'
                },
                {
                  label: t('to'),
                  name: 'to',
                  type: 'string',
                  isHexValue: true
                },
                {
                  label: t('type'),
                  name: 'type',
                  type: 'string'
                },
                {
                  label: t('domain'),
                  name: 'domain',
                  type: 'string'
                }
              ]}
              filters={filters}
              setFilters={setFilters}
            />
          </Box>
          <Box
            id="scrollableDivSubmissions"
            sx={{
              paddingRight: "15px",
              height: "calc(100vh - 250px)",
              ...getAltModeScrollBarStyle(theme.palette.mode)
            }}
          >
            {
              transactions?.map(transaction => (
                <PaladinTransaction
                  key={transaction.id}
                  paladinTransaction={transaction}
                />
              )
              )}
            {transactions?.length === 0 ?
              <Typography color="textSecondary" align="center" variant="h6" sx={{ marginTop: '40px' }}>
                {t(section === 'pending' ? 'noPendingTransactions' : 'noFailedTransactions')}
              </Typography>
              :
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
              />}
          </Box>
        </Box>
      </Fade>
      <TransactionLookupDialog
        dialogOpen={lookupTransactionDialogOpen}
        setDialogOpen={setLookupTransactionDialogOpen}
        label={t('submissionId')}
      />
    </>
  );
};
