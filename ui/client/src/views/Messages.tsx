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

import { Alert, Box, Button, Collapse, Fade, IconButton, Paper, Table, TableBody, TableCell, TableContainer, TableHead, TablePagination, TableRow, TableSortLabel, Tooltip, Typography } from "@mui/material";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { Dispatch, SetStateAction, useEffect, useState } from "react";
import { Timestamp } from "../components/Timestamp";
import { Tag } from "lucide-react";
import { customNavigate } from "../utils";
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import { Hash } from "../components/Hash";
import { queryMessages } from "../queries/transport";
import { Filters } from "../components/Filters";
import { IFilter } from "../interfaces";
import { constants } from "../components/config";
import { FiltersButton } from "../components/FiltersButton";
import { MessageLookupDialog } from "../dialogs/MessageLookup";
import SearchIcon from '@mui/icons-material/Search';

type Props = {
  sortAscending: boolean
  setSortAscending: Dispatch<SetStateAction<boolean>>
  refTimestamps: string[]
  setRefTimestamps: Dispatch<SetStateAction<string[]>>
  page: number
  setPage: Dispatch<SetStateAction<number>>
  rowsPerPage: number
  setRowsPerPage: Dispatch<SetStateAction<number>>
  filters: IFilter[]
  setFilters: Dispatch<SetStateAction<IFilter[]>>
  sortBy: string
  setSortBy: Dispatch<SetStateAction<string>>
};

export const Messages: React.FC<Props> = ({
  sortAscending,
  setSortAscending,
  refTimestamps,
  setRefTimestamps,
  page,
  setPage,
  rowsPerPage,
  setRowsPerPage,
  filters,
  setFilters,
  sortBy,
  setSortBy
}) => {

  const [lookupMessageDialogOpen, setLookupMessageDialogOpen] = useState(false);
  const [filtersVisible, setFiltersVisible] = useState(false);
  const [count, setCount] = useState(-1);
  const navigate = useNavigate();
  const { t } = useTranslation();

  const { data: messages, error } = useQuery({
    queryKey: ['messages', page, rowsPerPage, sortBy, sortAscending, filters],
    queryFn: () => queryMessages(rowsPerPage, sortBy, sortAscending, filters, refTimestamps[refTimestamps.length - 1]),
  });

  useEffect(() => {
    if (messages !== undefined && count === -1) {
      if (messages.length < rowsPerPage) {
        setCount(rowsPerPage * page + messages.length);
      }
    }
  }, [messages, rowsPerPage, page]);

  useEffect(() => {
    window.localStorage.setItem(constants.MESSAGES_FILTERS, JSON.stringify(filters));
  }, [filters]);

  if (messages === undefined) {
    return <></>
  }

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
      setRefTimestamps([]);
    } else if (newPage > page) {
      if (messages !== undefined) {
        const refEntriesCopy = [...refTimestamps];
        refEntriesCopy.push(messages[messages.length - 1].created);
        setRefTimestamps(refEntriesCopy);
      }
    } else {
      const refEntriesCopy = [...refTimestamps];
      refEntriesCopy.pop();
      setRefTimestamps(refEntriesCopy);
    }
    setPage(newPage);
  };

  const handleChangeRowsPerPage = (
    event: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>
  ) => {
    const value = parseInt(event.target.value, 10);
    setRowsPerPage(value);
    setRefTimestamps([]);
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
              {t("messages")}
            </Typography>
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
                    type: 'string'
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
          <Box sx={{
            display: 'flex',
            flexDirection: 'column',
            gap: '20px'
          }}>
            {messages !== undefined && messages.length > 0 &&
              <TableContainer
                component={Paper}
              >
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
                            setRefTimestamps([]);
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
                        }}>
                        <TableSortLabel
                          active={sortBy === 'acknowledged'}
                          direction={sortAscending ? 'asc' : 'desc'}
                          onClick={() => {
                            if (sortBy === 'ack.time') {
                              setSortAscending(!sortAscending);
                            } else {
                              setSortBy('ack.time')
                            }
                            setRefTimestamps([]);
                            setPage(0);
                          }}
                        >
                          {t('acknowledged')}
                        </TableSortLabel>
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
                        <TableCell>
                          <Timestamp timestamp={message.created} />
                        </TableCell>
                        <TableCell>
                          {message.ack?.time ?
                            <Timestamp timestamp={message.ack.time} />
                            :
                            <>--</>}
                        </TableCell>
                        <TableCell>
                          <Hash Icon={<Tag size="18px" />} title={t('id')} hash={message.id} />
                        </TableCell>
                        <TableCell>
                          {message.node}
                        </TableCell>
                        <TableCell>
                          {message.messageType}
                        </TableCell>
                        <TableCell align="right" sx={{ padding: '8px' }}>
                          <Tooltip title={t('open')} arrow>
                            <IconButton
                              onClick={mouseEvent => customNavigate(`/ui/messages/${message.id}`, mouseEvent, navigate)}>
                              <OpenInNewIcon color="secondary" fontSize="medium" />
                            </IconButton>
                          </Tooltip>
                        </TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                </Table>
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
              </TableContainer>}
          </Box>
        </Box>
      </Fade>
      <MessageLookupDialog
        dialogOpen={lookupMessageDialogOpen}
        setDialogOpen={setLookupMessageDialogOpen}
      />
    </>
  );

}