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

import { Alert, Box, Button, Collapse, Fade, IconButton, Paper, Table, TableBody, TableCell, TableContainer, TableHead, TablePagination, TableRow, TableSortLabel, ToggleButton, ToggleButtonGroup, Tooltip, Typography } from "@mui/material";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useApplicationContext } from "../contexts/ApplicationContext";
import { fetchSubmissions, buildPaladinTransactionPagingReference } from "../queries/transactions";
import { useTranslation } from "react-i18next";
import { Filters } from "../components/Filters";
import SearchIcon from '@mui/icons-material/Search';
import { TransactionLookupDialog } from "../dialogs/TransactionLookup";
import { FiltersButton } from "../components/FiltersButton";
import { Timestamp } from "../components/Timestamp";
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import { Hash } from "../components/Hash";
import { Tag } from "lucide-react";
import { customNavigate } from "../utils";
import { useNavigate } from "react-router-dom";
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import { pagedTableCount, useResetPaginationOnChange } from "../hooks/pagination";
import { AppRouteFactory } from '../routes';

export const Submissions: React.FC = () => {
  const { submissions } = useApplicationContext();
  const {
    section,
    setSection,
    page,
    setPage,
    rowsPerPage,
    setRowsPerPage,
    refEntries,
    setRefEntries,
    sortAscending,
    setSortAscending,
    filters,
    setFilters,
    filtersVisible,
    setFiltersVisible,
  } = submissions;

  const navigate = useNavigate();
  const [lookupTransactionDialogOpen, setLookupTransactionDialogOpen] = useState(false);
  const { t } = useTranslation();

  const { data, error, isPlaceholderData, isFetching } = useQuery({
    queryKey: ['submissions', rowsPerPage, section, filters, sortAscending, refEntries, rowsPerPage, page],
    queryFn: () => fetchSubmissions({
      type: section,
      limit: rowsPerPage,
      filters,
      sortAscending,
      pageParam: refEntries[refEntries.length - 1],
    }),
    placeholderData: keepPreviousData
  });

  const transactions = data?.items;
  const hasMore = data?.hasMore ?? false;

  const count = pagedTableCount(data, hasMore, page, rowsPerPage);

  useResetPaginationOnChange(() => {
    setRefEntries([]);
    setPage(0);
  }, filters);

  const handleChangePage = (
    _event: React.MouseEvent<HTMLButtonElement> | null,
    newPage: number
  ) => {
    if (newPage === 0) {
      setRefEntries([]);
    } else if (newPage > page) {
      if (transactions !== undefined && !isPlaceholderData && transactions.length > 0) {
        const refEntriesCopy = [...refEntries];
        refEntriesCopy.push(buildPaladinTransactionPagingReference(transactions[transactions.length - 1]));
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
            maxWidth: "1500px",
            marginLeft: "auto",
            marginRight: "auto",
          }}
        >
          <Box sx={{ display: 'flex', alignItems: 'center', gap: '20px', marginBottom: '20px', flexWrap: 'wrap' }}>
            <Typography variant="h5">
              {t("submissions")}
            </Typography>
            <ToggleButtonGroup size="small" sx={{ height: '30px' }} exclusive onChange={(_event, value) => {
              if (value !== null) {
                setPage(0);
                setRefEntries([]);
                setSection(value);
              }
            }} value={section}>
              <ToggleButton color="primary" value="pending" sx={{ width: '120px' }}>{t('pending')}</ToggleButton>
              <ToggleButton color="primary" value="failed" sx={{ width: '120px' }}>{t('failed')}</ToggleButton>
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
                    label: t('created'),
                    name: 'created',
                    type: 'timestamp',
                    isNanoSeconds: true
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
                  },
                  {
                    label: t('id'),
                    name: 'id',
                    type: 'string',
                    isUUID: true
                  },
                  {
                    label: t('from'),
                    name: 'from',
                    type: 'string'
                  },
                  {
                    label: t('to'),
                    name: 'to',
                    type: 'string'
                  }
                ]}
                filters={filters}
                setFilters={setFilters}
              />
            </Box>
          </Collapse>
          <Box>
            {transactions !== undefined && transactions.length > 0 &&
              <Paper>
                <TableContainer>
                  <Table stickyHeader>
                    <TableHead>
                      <TableRow>
                        <TableCell
                          width={1}
                          sx={{
                            backgroundColor: (theme) => theme.palette.background.paper,
                          }}>
                          <TableSortLabel
                            active={true}
                            direction={sortAscending ? 'asc' : 'desc'}
                            onClick={() => {
                              setSortAscending(!sortAscending);
                              setRefEntries([]);
                              setPage(0);
                            }}
                          >
                            {t('created')}
                          </TableSortLabel>
                        </TableCell>
                        <TableCell
                          width={1}
                          sx={{
                            backgroundColor: (theme) => theme.palette.background.paper,
                            whiteSpace: 'nowrap'
                          }}
                        >
                          {t('type')}
                        </TableCell>
                        <TableCell
                          width={1}
                          sx={{
                            backgroundColor: (theme) => theme.palette.background.paper,
                            whiteSpace: 'nowrap'
                          }}
                        >
                          {t('domain')}
                        </TableCell>
                        <TableCell
                          width={1}
                          sx={{
                            backgroundColor: (theme) => theme.palette.background.paper,
                            whiteSpace: 'nowrap'
                          }}
                        >
                          {t('id')}
                        </TableCell>

                        <TableCell
                          width={1}
                          sx={{
                            backgroundColor: (theme) => theme.palette.background.paper,
                            whiteSpace: 'nowrap'
                          }}
                        >
                          {t('from')}
                        </TableCell>

                        <TableCell
                          width={1}
                          sx={{
                            backgroundColor: (theme) => theme.palette.background.paper,
                            whiteSpace: 'nowrap'
                          }}
                        >
                          {t('to')}
                        </TableCell>

                        <TableCell
                          width={'100%'}
                          sx={{
                            backgroundColor: (theme) => theme.palette.background.paper,
                            whiteSpace: 'nowrap'
                          }}
                        >
                        </TableCell>
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {transactions.map(transaction =>
                        <TableRow key={transaction.id}>
                          <TableCell sx={{ paddingTop: '8px', paddingBottom: '8px' }}>
                            <Timestamp timestamp={transaction.created} />
                          </TableCell>
                          <TableCell>
                            {t(transaction.type)}
                          </TableCell>
                          <TableCell>
                            {transaction.domain ? t(transaction.domain) : '--'}
                          </TableCell>
                          <TableCell sx={{ paddingTop: '8px', paddingBottom: '8px' }}>
                            <Hash Icon={<Tag size="18px" />} hideTitle title={t('id')} hash={transaction.id} />
                          </TableCell>
                          <TableCell sx={{ paddingTop: '8px', paddingBottom: '8px' }}>
                            <Hash Icon={<Tag size="18px" />} hideTitle title={t('id')} hash={transaction.from} />
                          </TableCell>
                          <TableCell sx={{ paddingTop: '8px', paddingBottom: '8px' }}>
                            <Hash Icon={<Tag size="18px" />} hideTitle title={t('id')} hash={transaction.to ?? '--'} />
                          </TableCell>
                          <TableCell align="right" sx={{ paddingTop: '8px', paddingBottom: '8px' }}>
                            <Tooltip title={t('open')} arrow>
                              <IconButton
                                onClick={mouseEvent => customNavigate(AppRouteFactory.getPath('Transaction', { hashOrId: transaction.id }, { back: 'submissions' }), mouseEvent, navigate)}>
                                <OpenInNewIcon color="secondary" fontSize="medium" />
                              </IconButton>
                            </Tooltip>
                          </TableCell>
                        </TableRow>
                      )}
                    </TableBody>
                  </Table>
                </TableContainer>
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
                />
              </Paper>}
            {transactions !== undefined && transactions.length === 0 &&
              <Box sx={{ marginTop: '20px', textAlign: 'center', color: theme => theme.palette.text.secondary }}>
                <InfoOutlinedIcon sx={{ fontSize: '50px' }} />
                <Typography>{t('noSubmissions')}</Typography>
              </Box>}
          </Box>
        </Box>
      </Fade>
      {lookupTransactionDialogOpen && (
        <TransactionLookupDialog
          onClose={() => setLookupTransactionDialogOpen(false)}
          label={t('submissionId')}
          backToSubmissions={true}
        />
      )}
    </>
  );
};
