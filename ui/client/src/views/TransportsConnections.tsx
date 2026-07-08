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

import { Alert, Box, Button, Collapse, Fade, Paper, Table, TableBody, TableCell, TableContainer, TableHead, TablePagination, TableRow, TableSortLabel, ToggleButton, ToggleButtonGroup, Typography } from "@mui/material";
import { useTranslation } from "react-i18next";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { useApplicationContext } from "../contexts/ApplicationContext";
import { Timestamp } from "../components/Timestamp";
import { fetchTransportPeersWithQuery } from "../queries/transport";
import { Filters } from "../components/Filters";
import { FiltersButton } from "../components/FiltersButton";
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import prettyBytes from "pretty-bytes";
import RefreshIcon from '@mui/icons-material/Refresh';
import { useLocation, useNavigate } from "react-router-dom";
import { AppRoutes } from "../routes";

export const TransportConnections: React.FC = () => {
  const { transports: transportsViewState } = useApplicationContext();
  const {
    sortAscending,
    setSortAscending,
    refNames,
    setRefNames,
    page,
    setPage,
    rowsPerPage,
    setRowsPerPage,
    filters,
    setFilters,
    filtersVisible,
    setFiltersVisible,
  } = transportsViewState;

  const [count, setCount] = useState(-1);
  const { t } = useTranslation();
  const navigate = useNavigate();
  const location = useLocation();

  const { data, error, refetch, isPlaceholderData, isFetching } = useQuery({
    queryKey: ['transports', page, rowsPerPage, sortAscending, filters, refNames],
    queryFn: () => fetchTransportPeersWithQuery(rowsPerPage, sortAscending, filters, refNames[refNames.length - 1]),
    placeholderData: keepPreviousData
  });

  const peers = data?.items;
  const hasMore = data?.hasMore ?? false;

  useEffect(() => {
    if (data !== undefined && count === -1 && !isPlaceholderData && !data.hasMore) {
      setCount(rowsPerPage * page + data.items.length);
    }
  }, [data, rowsPerPage, page, isPlaceholderData]);

  useEffect(() => {
    setRefNames([]);
    setPage(0);
    setCount(-1);
  }, [filters]);

  if (error) {
    return (<Alert sx={{ margin: '30px' }} severity="error" variant="filled">
      {error?.message}
    </Alert>);
  }

  const handleChangePage = (
    _event: React.MouseEvent<HTMLButtonElement> | null,
    newPage: number
  ) => {
    if (newPage === 0) {
      setRefNames([]);
    } else if (newPage > page) {
      if (peers !== undefined && !isPlaceholderData && peers.length > 0) {
        const refEntriesCopy = [...refNames];
        refEntriesCopy.push(peers[peers.length - 1].name);
        setRefNames(refEntriesCopy);
      }
    } else {
      const refEntriesCopy = [...refNames];
      refEntriesCopy.pop();
      setRefNames(refEntriesCopy);
    }
    setPage(newPage);
  };

  const handleChangeRowsPerPage = (
    event: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>
  ) => {
    const value = parseInt(event.target.value, 10);
    setRowsPerPage(value);
    setRefNames([]);
    setPage(0);
  };

  return (
    <>
      <Fade timeout={location.state?.skipFade === true? 0 : 600} in={true}>
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
              {t("transports")}
            </Typography>
            <ToggleButtonGroup size="small" sx={{ height: '30px' }} exclusive value="connections">
              <ToggleButton color="primary" value="connections" sx={{ width: '120px' }}>{t('connections')}</ToggleButton>
              <ToggleButton color="primary" value="messages" sx={{ width: '120px' }} onClick={() => navigate(AppRoutes.TransportMessages, { state: { skipFade: true } })}>{t('messages')}</ToggleButton>
            </ToggleButtonGroup>
            <Box sx={{ flexGrow: 1, display: 'flex', justifyContent: 'right', gap: '10px' }}>
              <Button
                sx={{ borderRadius: '20px', minWidth: '120px' }}
                size="small"
                variant="outlined"
                startIcon={<RefreshIcon />}
                onClick={() => refetch()}
              >
                {t('refresh')}
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
                    label: t('node'),
                    name: 'name',
                    type: 'string'
                  },
                  {
                    label: t('activated'),
                    name: 'activated',
                    type: 'timestamp',
                    isNanoSeconds: true
                  },
                  {
                    label: t('lastSend'),
                    name: 'stats.lastSend',
                    type: 'timestamp',
                    isNanoSeconds: true
                  },
                  {
                    label: t('lastReceive'),
                    name: 'stats.lastReceive',
                    type: 'timestamp',
                    isNanoSeconds: true
                  },
                  {
                    label: t('messagesSent'),
                    name: 'stats.sentMsgs',
                    type: 'number',
                  },
                  {
                    label: t('messagesReceived'),
                    name: 'stats.receivedMsgs',
                    type: 'number',
                  }
                ]}
                filters={filters}
                setFilters={setFilters}
              />
            </Box>
          </Collapse>
          {peers !== undefined && peers.length > 0 &&
            <Paper>
              <TableContainer>
                <Table stickyHeader>
                  <TableHead>
                    <TableRow>
                      <TableCell
                        width={1}
                        sx={{
                          backgroundColor: (theme) => theme.palette.background.paper,
                          whiteSpace: 'nowrap'
                        }}>
                        <TableSortLabel
                          active={true}
                          direction={sortAscending ? 'asc' : 'desc'}
                          onClick={() => {
                            setSortAscending(!sortAscending);
                            setRefNames([]);
                            setPage(0);
                          }}
                        >
                          {t('node')}
                        </TableSortLabel>
                      </TableCell>
                      <TableCell
                        width={1}
                        sx={{
                          backgroundColor: (theme) => theme.palette.background.paper,
                          whiteSpace: 'nowrap'
                        }}
                      >
                        {t('activated')}
                      </TableCell>
                      <TableCell
                        width={1}
                        sx={{
                          backgroundColor: (theme) => theme.palette.background.paper,
                          whiteSpace: 'nowrap'
                        }}
                      >
                        {t('lastSend')}
                      </TableCell>
                      <TableCell
                        width={1}
                        sx={{
                          backgroundColor: (theme) => theme.palette.background.paper,
                          whiteSpace: 'nowrap'
                        }}
                      >
                        {t('lastReceive')}
                      </TableCell>
                      <TableCell
                        width={1}
                        sx={{
                          backgroundColor: (theme) => theme.palette.background.paper,
                          whiteSpace: 'nowrap'
                        }}
                      >
                        {t('messagesSent')}
                      </TableCell>
                      <TableCell
                        width={1}
                        sx={{
                          backgroundColor: (theme) => theme.palette.background.paper,
                          whiteSpace: 'nowrap'
                        }}
                      >
                        {t('messagesReceived')}
                      </TableCell>
                      <TableCell
                        width={1}
                        sx={{
                          backgroundColor: (theme) => theme.palette.background.paper,
                          whiteSpace: 'nowrap'
                        }}
                      >
                        {t('dataSent')}
                      </TableCell>
                      <TableCell
                        width={'100%'}
                        sx={{
                          backgroundColor: (theme) => theme.palette.background.paper,
                          whiteSpace: 'nowrap'
                        }}
                      >
                        {t('dataReceived')}
                      </TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {peers.map(peer =>
                      <TableRow key={peer.name}>
                        <TableCell>
                          {peer.name}
                        </TableCell>
                        <TableCell sx={{ paddingTop: '8px', paddingBottom: '8px' }}>
                          <Timestamp timestamp={peer.stats.createdAt} />
                        </TableCell>
                        <TableCell sx={{ paddingTop: '8px', paddingBottom: '8px' }}>
                          {peer.stats.lastSend ?
                            <Timestamp timestamp={peer.stats.lastSend} />
                            :
                            <>--</>}
                        </TableCell>
                        <TableCell sx={{ paddingTop: '8px', paddingBottom: '8px' }}>
                          {peer.stats.lastReceive ?
                            <Timestamp timestamp={peer.stats.lastReceive} />
                            :
                            <>--</>}
                        </TableCell>
                        <TableCell>
                          {peer.stats.sentMsgs.toLocaleString()}
                        </TableCell>
                        <TableCell>
                          {peer.stats.receivedMsgs.toLocaleString()}
                        </TableCell>
                        <TableCell>
                          {prettyBytes(peer.stats.sentBytes)}
                        </TableCell>
                        <TableCell>
                          {prettyBytes(peer.stats.receivedBytes)}
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
          {peers !== undefined && peers.length === 0 &&
            <Box sx={{ marginTop: '20px', textAlign: 'center', color: theme => theme.palette.text.secondary }}>
              <InfoOutlinedIcon sx={{ fontSize: '50px' }} />
              <Typography>{t('peersEmptyState')}</Typography>
            </Box>
          }
        </Box>
      </Fade>
    </>
  );

}