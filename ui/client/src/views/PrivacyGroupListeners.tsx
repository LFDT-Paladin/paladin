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
import { useEffect, useState } from "react";
import { useApplicationContext } from "../contexts/ApplicationContext";
import { useTranslation } from "react-i18next";
import { listPrivacyGroupListeners } from "../queries/privacyGroups";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { Timestamp } from "../components/Timestamp";
import { useLocation, useNavigate } from "react-router-dom";
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import { FiltersButton } from "../components/FiltersButton";
import { Filters } from "../components/Filters";
import { AppRoutes } from "../routes";
import CircleIcon from '@mui/icons-material/Circle';
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import { customNavigate } from "../utils";
import AddIcon from '@mui/icons-material/Add';
import { CreatePrivacyGroupListenerDialog } from "../dialogs/CreatePrivacyGroupListener";
import { PrivacyGroupListenerActions } from "../components/PrivacyGroupListenerActions";
import { Hash } from "../components/Hash";
import { Captions } from "lucide-react";

export const PrivacyGroupListeners: React.FC = () => {
  const { privacyGroupListeners: privacyGroupListenersViewState, readOnly } = useApplicationContext();
  const {
    sortAscending,
    setSortAscending,
    paginationRefs,
    setPaginationRefs,
    sortBy,
    setSortBy,
    page,
    setPage,
    rowsPerPage,
    setRowsPerPage,
    filters,
    setFilters,
    filtersVisible,
    setFiltersVisible,
  } = privacyGroupListenersViewState;

  const [createPrivacyGroupListenerDialogOpen, setCreatePrivacyGroupListenerDialogOpen] = useState(false);
  const navigate = useNavigate();
  const [count, setCount] = useState(-1);
  const { t } = useTranslation();
  const location = useLocation();

  const { data, error, isPlaceholderData, isFetching, refetch } = useQuery({
    queryKey: ['privacyGroups', page, rowsPerPage, filters, sortBy, sortAscending],
    queryFn: () => listPrivacyGroupListeners(rowsPerPage, filters, sortBy, sortAscending, paginationRefs[paginationRefs.length - 1]),
    placeholderData: keepPreviousData
  });

  const privacyGroupListeners = data?.items;
  const hasMore = data?.hasMore ?? false;

  useEffect(() => {
    if (data !== undefined && count === -1 && !isPlaceholderData && !data.hasMore) {
      setCount(rowsPerPage * page + data.items.length);
    }
  }, [data, rowsPerPage, page, isPlaceholderData]);

  if (error) {
    return (
      <Alert sx={{ margin: '30px' }} severity="error" variant="filled">
        {error.message}
      </Alert>
    );
  }

  const handleChangePage = (
    _event: React.MouseEvent<HTMLButtonElement> | null,
    newPage: number
  ) => {
    if (newPage === 0) {
      setPaginationRefs([]);
    } else if (newPage > page) {
      if (privacyGroupListeners !== undefined && !isPlaceholderData && privacyGroupListeners.length > 0) {
        const refEntriesCopy = [...paginationRefs];
        refEntriesCopy.push(privacyGroupListeners[privacyGroupListeners.length - 1].created);
        setPaginationRefs(refEntriesCopy);
      }
    } else {
      const refEntriesCopy = [...paginationRefs];
      refEntriesCopy.pop();
      setPaginationRefs(refEntriesCopy);
    }
    setPage(newPage);
  };

  const handleChangeRowsPerPage = (
    event: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>
  ) => {
    const value = parseInt(event.target.value, 10);
    setRowsPerPage(value);
    setPaginationRefs([]);
    setPage(0);
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
            <Typography align="center" variant="h5">
              {t("privacyGroups")}
            </Typography>
            <ToggleButtonGroup size="small" sx={{ height: '30px' }} exclusive value="listeners">
              <ToggleButton color="primary" value="groups" sx={{ width: '120px' }} onClick={() => navigate(AppRoutes.PrivacyGroups, { state: { skipFade: true } })}>{t('groups')}</ToggleButton>
              <ToggleButton color="primary" value="listeners" sx={{ width: '120px' }}>{t('listeners')}</ToggleButton>
            </ToggleButtonGroup>
            <Box sx={{ flexGrow: 1, display: 'flex', justifyContent: 'right', gap: '10px' }}>
              {!readOnly &&
                <Button
                  sx={{ borderRadius: '20px', minWidth: '120px' }}
                  size="small"
                  variant="outlined"
                  startIcon={<AddIcon />}
                  onClick={() => setCreatePrivacyGroupListenerDialogOpen(true)}
                >
                  {t('create')}
                </Button>}
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
                    label: t('name'),
                    name: 'name',
                    type: 'string',
                  },
                  {
                    label: t('created'),
                    name: 'created',
                    type: 'timestamp',
                    isNanoSeconds: true
                  },
                  {
                    label: t('started'),
                    name: 'started',
                    type: 'boolean'
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
            {privacyGroupListeners !== undefined && privacyGroupListeners.length > 0 &&
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
                            active={sortBy === 'name'}
                            direction={sortAscending ? 'asc' : 'desc'}
                            onClick={() => {
                              if (sortBy === 'name') {
                                setSortAscending(!sortAscending);
                              } else {
                                setSortBy('name');
                              }
                              setPaginationRefs([]);
                              setPage(0);
                            }}
                          >
                            {t('name')}
                          </TableSortLabel>
                        </TableCell>
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
                              setPaginationRefs([]);
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
                            whiteSpace: 'nowrap',
                            minWidth: '120px'
                          }}
                        >
                          {t('status')}
                        </TableCell>

                        <TableCell
                          width={1}
                          sx={{
                            backgroundColor: (theme) => theme.palette.background.paper,
                            whiteSpace: 'nowrap',
                            minWidth: '120px'
                          }}
                        >
                          {t('domain')}
                        </TableCell>
                        <TableCell
                          width={1}
                          sx={{
                            backgroundColor: (theme) => theme.palette.background.paper,
                            whiteSpace: 'nowrap',
                            minWidth: '120px'
                          }}
                        >
                          {t('group')}
                        </TableCell>
                        <TableCell
                          width={1}
                          sx={{
                            backgroundColor: (theme) => theme.palette.background.paper,
                            whiteSpace: 'nowrap',
                            minWidth: '120px'
                          }}
                        >
                          {t('topic')}
                        </TableCell>

                        <TableCell
                          sx={{
                            backgroundColor: (theme) => theme.palette.background.paper,
                            whiteSpace: 'nowrap',
                            width: '1'
                          }}
                        >
                          {readOnly ? '' : t('actions')}
                        </TableCell>
                        <TableCell
                          sx={{
                            backgroundColor: (theme) => theme.palette.background.paper,
                            width: '100%'
                          }}
                        />
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {privacyGroupListeners?.map(privacyGroupListener =>
                        <TableRow key={privacyGroupListener.name}>
                          <TableCell>
                            {privacyGroupListener.name}
                          </TableCell>
                          <TableCell sx={{ paddingTop: '8px', paddingBottom: '8px' }}>
                            <Timestamp timestamp={privacyGroupListener.created} />
                          </TableCell>
                          <TableCell>
                            <Box sx={{
                              display: 'flex',
                              alignItems: 'center',
                              gap: '8px'
                            }}>
                              <CircleIcon sx={{ fontSize: '16px' }} color={privacyGroupListener.started ? 'success' : 'warning'} />
                              <Typography variant="body2">
                                {t(privacyGroupListener.started ? 'started' : 'stopped')}
                              </Typography>
                            </Box>
                          </TableCell>
                          <TableCell>
                            {privacyGroupListener.filters.domain ?? '--'}
                          </TableCell>
                          <TableCell>
                            {privacyGroupListener.filters.group?
                            <Hash Icon={<Captions size="18px" />} title={t('group')} hideTitle hash={privacyGroupListener.filters.group} />
                            :
                            '--'}
                          </TableCell>
                          <TableCell>
                            {privacyGroupListener.filters.topic ?? '--'}
                          </TableCell>
                          <TableCell sx={{ padding: '8px' }}>
                            <PrivacyGroupListenerActions
                              privacyGroupListener={privacyGroupListener}
                              refetch={refetch}
                            />
                          </TableCell>
                          <TableCell sx={{ padding: '8px' }}>
                            <Tooltip title={t('open')} arrow>
                              <IconButton
                                onClick={mouseEvent => customNavigate(`/ui/privacy-groups/listeners/${privacyGroupListener.name}`, mouseEvent, navigate)}>
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
            {privacyGroupListeners !== undefined && privacyGroupListeners.length === 0 &&
              <Box sx={{ marginTop: '20px', textAlign: 'center', color: theme => theme.palette.text.secondary }}>
                <InfoOutlinedIcon sx={{ fontSize: '50px' }} />
                <Typography>{t('privacyGroupListenersEmptyState')}</Typography>
              </Box>
            }
          </Box>
        </Box>
      </Fade>
      <CreatePrivacyGroupListenerDialog
        dialogOpen={createPrivacyGroupListenerDialogOpen}
        setDialogOpen={setCreatePrivacyGroupListenerDialogOpen}
      />
    </>
  );

}