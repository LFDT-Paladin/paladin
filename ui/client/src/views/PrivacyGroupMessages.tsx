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
import { useTranslation } from "react-i18next";
import { getPrivacyGroupMessages, buildPrivacyGroupMessagePagingReference } from "../queries/privacyGroups";
import { useApplicationContext } from "../contexts/ApplicationContext";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { FiltersButton } from "../components/FiltersButton";
import SearchIcon from '@mui/icons-material/Search';
import { useState } from "react";
import { Filters } from "../components/Filters";
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import { Tag } from "lucide-react";
import { Timestamp } from "../components/Timestamp";
import { Hash } from "../components/Hash";
import { customNavigate } from "../utils";
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import { useLocation, useNavigate } from "react-router-dom";
import { PrivacyGroupMessageLookupDialog } from "../dialogs/PrivacyGroupMessageLookup";
import SendIcon from '@mui/icons-material/Send';
import { SendPrivacyGroupMessageDialog } from "../dialogs/SendPrivacyGroupMessage";
import { AppRoutes } from "../routes";
import { pagedTableCount, useResetPaginationOnChange } from "../hooks/pagination";

export const PrivacyGroupMessages: React.FC = () => {

  const [sendMessageDialogOpen, setSendMessageDialogOpen] = useState(false);
  const [lookupPrivacyGroupMessageDialogOpen, setLookupPrivacyGroupMessageDialogOpen] = useState(false);
  const navigate = useNavigate();
  const location = useLocation();
  const { privacyGroupMessages: privacyGroupMessagesViewStateState, readOnly } = useApplicationContext();
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
    filtersVisible,
    setFiltersVisible,
  } = privacyGroupMessagesViewStateState;
  const { t } = useTranslation();

  const { data, error, isPlaceholderData, isFetching } = useQuery({
    queryKey: ['privacy-group-messages', rowsPerPage, filters, sortAscending, refEntries],
    queryFn: () => getPrivacyGroupMessages({
      limit: rowsPerPage,
      filters,
      sortAscending,
      pageRef: refEntries[refEntries.length - 1],
    }),
    placeholderData: keepPreviousData
  });

  const privacyGroupMessages = data?.items;
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
      if (privacyGroupMessages !== undefined && !isPlaceholderData && privacyGroupMessages.length > 0) {
        const refEntriesCopy = [...refEntries];
        refEntriesCopy.push(buildPrivacyGroupMessagePagingReference(privacyGroupMessages[privacyGroupMessages.length - 1]));
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

  const getPreselectedPrivacyGroup = () => {
    const filterEntry = filters.find(filter => filter.field.name === 'group' && filter.operator === 'equal');
    if(filterEntry !== undefined) {
      return filterEntry.value.toString();
    }
  };

  return (
    <>
      <Fade timeout={location.state?.skipFade === true ? 0 : 600} in={true}>
        <Box
          sx={{
            padding: "20px",
            maxWidth: "1500px",
            marginLeft: "auto",
            marginRight: "auto",
          }}
        >
          <Box sx={{ display: 'flex', alignItems: 'center', gap: '20px', marginBottom: '20px', flexWrap: 'wrap' }}>
            <Typography variant="h5">{t('privacyGroups')}</Typography>
            <ToggleButtonGroup size="small" sx={{ height: '30px' }} exclusive value="messages">
              <ToggleButton color="primary" value="groups" sx={{ width: '120px' }} onClick={() => navigate(AppRoutes.PrivacyGroups, { state: { skipFade: true } })}>{t('groups')}</ToggleButton>
              <ToggleButton color="primary" value="messages" sx={{ width: '120px' }}>{t('messages')}</ToggleButton>
            </ToggleButtonGroup>
            <Box sx={{ flexGrow: 1, display: 'flex', justifyContent: 'right', gap: '10px' }}>
              {!readOnly &&
              <Button
                sx={{ borderRadius: '20px', minWidth: '120px' }}
                size="small"
                variant="outlined"
                startIcon={<SendIcon />}
                onClick={() => setSendMessageDialogOpen(true)}
              >
                {t('send')}
              </Button>}
              <Button
                sx={{ borderRadius: '20px', minWidth: '120px' }}
                size="small"
                variant="outlined"
                startIcon={<SearchIcon />}
                onClick={() => setLookupPrivacyGroupMessageDialogOpen(true)}
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
                    label: t('sent'),
                    name: 'sent',
                    type: 'timestamp',
                    isNanoSeconds: true
                  },
                  {
                    label: t('received'),
                    name: 'received',
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
                    label: t('group'),
                    name: 'group',
                    type: 'string',
                    isHexValue: true
                  },
                  {
                    label: t('node'),
                    name: 'node',
                    type: 'string'
                  },
                  {
                    label: t('domain'),
                    name: 'domain',
                    type: 'string'
                  },
                  {
                    label: t('topic'),
                    name: 'topic',
                    type: 'string'
                  },
                  {
                    label: t('localSequence'),
                    name: 'localSequence',
                    type: 'number'
                  }
                ]}
                filters={filters}
                setFilters={setFilters}
              />
            </Box>
          </Collapse>
          {privacyGroupMessages !== undefined && privacyGroupMessages.length > 0 &&
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
                          {t('sent')}
                        </TableSortLabel>
                      </TableCell>
                      <TableCell
                        width={1}
                        sx={{
                          backgroundColor: (theme) => theme.palette.background.paper,
                          whiteSpace: 'nowrap'
                        }}
                      >
                        {t('received')}
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
                        {t('group')}
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
                        {t('topic')}
                      </TableCell>
                      <TableCell
                        width={'100%'}
                        sx={{
                          backgroundColor: (theme) => theme.palette.background.paper,
                          whiteSpace: 'nowrap'
                        }}
                      >
                        {t('localSequence')}
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
                    {privacyGroupMessages.map(privacyGroupMessage =>
                      <TableRow key={privacyGroupMessage.id}>
                        <TableCell sx={{ paddingTop: '8px', paddingBottom: '8px' }}>
                          <Timestamp timestamp={privacyGroupMessage.sent} />
                        </TableCell>
                        <TableCell sx={{ paddingTop: '8px', paddingBottom: '8px' }}>
                          {privacyGroupMessage.received ?
                            <Timestamp timestamp={privacyGroupMessage.received} />
                            :
                            <>--</>}
                        </TableCell>
                        <TableCell>
                          <Hash Icon={<Tag size="18px" />} hideTitle title={t('id')} hash={privacyGroupMessage.id} />
                        </TableCell>
                        <TableCell>
                          <Hash Icon={<Tag size="18px" />} hideTitle title={t('group')} hash={privacyGroupMessage.group} />
                        </TableCell>
                        <TableCell sx={{ whiteSpace: 'nowrap' }}>
                          {privacyGroupMessage.node}
                        </TableCell>
                        <TableCell sx={{ whiteSpace: 'nowrap' }}>
                          {privacyGroupMessage.domain}
                        </TableCell>
                        <TableCell sx={{ whiteSpace: 'nowrap' }}>
                          {privacyGroupMessage.topic}
                        </TableCell>
                        <TableCell>
                          {privacyGroupMessage.localSequence}
                        </TableCell>
                        <TableCell align="right" sx={{ paddingTop: '8px', paddingBottom: '8px' }}>
                          <Tooltip title={t('open')} arrow>
                            <IconButton
                              onClick={mouseEvent => customNavigate(`/ui/privacy-groups/messages/${privacyGroupMessage.id}`, mouseEvent, navigate)}>
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
          {privacyGroupMessages !== undefined && privacyGroupMessages.length === 0 &&
            <Box sx={{ marginTop: '20px', textAlign: 'center', color: theme => theme.palette.text.secondary }}>
              <InfoOutlinedIcon sx={{ fontSize: '50px' }} />
              <Typography>{t('privacyGroupMessagesEmptyState')}</Typography>
            </Box>
          }
          {sendMessageDialogOpen && (
            <SendPrivacyGroupMessageDialog
              preSelectedPrivacyGroupId={getPreselectedPrivacyGroup()}
              onClose={() => setSendMessageDialogOpen(false)}
            />
          )}
          {lookupPrivacyGroupMessageDialogOpen && (
            <PrivacyGroupMessageLookupDialog
              onClose={() => setLookupPrivacyGroupMessageDialogOpen(false)}
            />
          )}
        </Box>
      </Fade>
    </>
  );
}