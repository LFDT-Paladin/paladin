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

import { Alert, Box, Button, Collapse, Fade, IconButton, Paper, Table, TableBody, TableCell, TableContainer, TableHead, TablePagination, TableRow, TableSortLabel, ToggleButton, ToggleButtonGroup, Tooltip, Typography } from "@mui/material";
import { useTranslation } from "react-i18next";
import { useLocation, useNavigate } from "react-router-dom";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useApplicationContext } from "../contexts/ApplicationContext";
import { Timestamp } from "../components/Timestamp";
import { Tag } from "lucide-react";
import { customNavigate } from "../utils";
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import { Hash } from "../components/Hash";
import { queryMessages, buildMessagePagingReference } from "../queries/transport";
import { Filters } from "../components/Filters";
import { FiltersButton } from "../components/FiltersButton";
import { ReliableMessageLookupDialog } from "../dialogs/ReliableMessageLookup";
import SearchIcon from '@mui/icons-material/Search';
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import { AppRoutes } from "../routes";
import { pagedTableCount, useResetPaginationOnChange } from "../hooks/pagination";

export const TransportMessages: React.FC = () => {
  const { messages: messagesViewState } = useApplicationContext();
  const {
    sortAscending,
    setSortAscending,
    refEntries,
    setRefEntries,
    page,
    setPage,
    rowsPerPage,
    setRowsPerPage,
    filters,
    setFilters,
    sortBy,
    setSortBy,
    filtersVisible,
    setFiltersVisible,
  } = messagesViewState;

  const [lookupMessageDialogOpen, setLookupMessageDialogOpen] = useState(false);
  const navigate = useNavigate();
  const { t } = useTranslation();
  const location = useLocation();

  const { data, error, isPlaceholderData, isFetching } = useQuery({
    queryKey: ['messages', page, rowsPerPage, sortBy, sortAscending, filters, refEntries],
    queryFn: () => queryMessages(rowsPerPage, sortBy, sortAscending, filters, refEntries[refEntries.length - 1]),
    placeholderData: keepPreviousData
  });

  const messages = data?.items;
  const hasMore = data?.hasMore ?? false;

  const count = pagedTableCount(data, hasMore, page, rowsPerPage);

  useResetPaginationOnChange(() => {
    setRefEntries([]);
    setPage(0);
  }, filters);

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
      setRefEntries([]);
    } else if (newPage > page) {
      if (messages !== undefined && !isPlaceholderData && messages.length > 0) {
        const refEntriesCopy = [...refEntries];
        refEntriesCopy.push(buildMessagePagingReference(messages[messages.length - 1], sortBy));
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
              {t('transports')}
            </Typography>
            <ToggleButtonGroup size="small" sx={{ height: '30px' }} exclusive value="messages">
              <ToggleButton color="primary" value="connections" sx={{ width: '120px' }} onClick={() => navigate(AppRoutes.TransportConnections, { state: { skipFade: true } })}>{t('connections')}</ToggleButton>
              <ToggleButton color="primary" value="messages" sx={{ width: '120px' }}>{t('messages')}</ToggleButton>
            </ToggleButtonGroup>
            <Box sx={{ flexGrow: 1, display: 'flex', justifyContent: 'right', gap: '10px' }}>
              <Button
                sx={{ borderRadius: '20px', minWidth: '120px' }}
                size="small"
                variant="outlined"
                startIcon={<SearchIcon />}
                onClick={() => setLookupMessageDialogOpen(true)}
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
                    label: t('acknowledged'),
                    name: 'ack.time',
                    type: 'timestamp',
                    isNanoSeconds: true
                  },
                  {
                    label: t('id'),
                    name: 'id',
                    type: 'string',
                    isUUID: true
                  },
                  {
                    label: t('node'),
                    name: 'node',
                    type: 'string'
                  },
                  {
                    label: t('type'),
                    name: 'messageType',
                    type: 'string'
                  }
                ]}
                filters={filters}
                setFilters={setFilters}
              />
            </Box>
          </Collapse>
          {messages !== undefined && messages.length > 0 &&
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
                          active={sortBy === 'created'}
                          direction={sortAscending ? 'asc' : 'desc'}
                          onClick={() => {
                            if (sortBy === 'created') {
                              setSortAscending(!sortAscending);
                            } else {
                              setSortBy('created');
                            }
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
                        {t('acknowledged')}
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
                        {t('node')}
                      </TableCell>
                      <TableCell
                        width={'100%'}
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
                      </TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {messages.map(message =>
                      <TableRow key={message.id}>
                        <TableCell sx={{ paddingTop: '8px', paddingBottom: '8px' }}>
                          <Timestamp timestamp={message.created} />
                        </TableCell>
                        <TableCell sx={{ paddingTop: '8px', paddingBottom: '8px' }}>
                          {message.ack?.time ?
                            <Timestamp timestamp={message.ack.time} />
                            :
                            <>--</>}
                        </TableCell>
                        <TableCell>
                          <Hash Icon={<Tag size="18px" />} hideTitle title={t('id')} hash={message.id} />
                        </TableCell>
                        <TableCell>
                          {message.node}
                        </TableCell>
                        <TableCell>
                          {message.messageType}
                        </TableCell>
                        <TableCell align="right" sx={{ paddingTop: '8px', paddingBottom: '8px' }}>
                          <Tooltip title={t('open')} arrow>
                            <IconButton
                              onClick={mouseEvent => customNavigate(`/ui/transports/messages/${message.id}`, mouseEvent, navigate)}>
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
          {messages !== undefined && messages.length === 0 &&
            <Box sx={{ marginTop: '20px', textAlign: 'center', color: theme => theme.palette.text.secondary }}>
              <InfoOutlinedIcon sx={{ fontSize: '50px' }} />
              <Typography>{t('messagesEmptyState')}</Typography>
            </Box>
          }
          <ReliableMessageLookupDialog
            dialogOpen={lookupMessageDialogOpen}
            setDialogOpen={setLookupMessageDialogOpen}
          />
        </Box>
      </Fade>
    </>
  );

}