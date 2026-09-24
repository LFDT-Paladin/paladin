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

import { Alert, Box, Button, Collapse, Fade, IconButton, MenuItem, Paper, Table, TableBody, TableCell, TableContainer, TableHead, TablePagination, TableRow, TableSortLabel, TextField, ToggleButton, ToggleButtonGroup, Tooltip, Typography } from "@mui/material";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { useApplicationContext } from "../contexts/ApplicationContext";
import { fetchRegistries, fetchRegistryEntries } from "../queries/registry";
import { useTranslation } from "react-i18next";
import PersonSearchIcon from '@mui/icons-material/PersonSearch';
import { ResolveVerifierDialog } from "../dialogs/ResolveVerifier";
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import { customNavigate } from "../utils";
import { useNavigate } from "react-router-dom";
import { Hash } from "../components/Hash";
import { Tag } from "lucide-react";
import { Captions } from "lucide-react";
import { FiltersButton } from "../components/FiltersButton";
import { Filters } from "../components/Filters";
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import { pagedTableCount, useResetPaginationOnChange } from "../hooks/pagination";
import { AppRouteFactory } from '../routes';

export const Registries: React.FC = () => {
  const { registry } = useApplicationContext();
  const {
    filters,
    setFilters,
    refNames,
    setRefNames,
    sortAscending,
    setSortAscending,
    page,
    setPage,
    rowsPerPage,
    setRowsPerPage,
    filtersVisible,
    setFiltersVisible,
  } = registry;

  const [activeFilter, setActiveFilter] = useState<'active' | 'inactive' | 'any'>('any');
  const [resolveVerifierDialogOpen, setResolveVerifierDialogOpen] = useState(false);
  const [selectedRegistry, setSelectedRegistry] = useState<string>();
  const navigate = useNavigate();
  const { t } = useTranslation();

  const { data: registries, error: registriesError } = useQuery({
    queryKey: ['registries'],
    queryFn: () => fetchRegistries()
  });

  useEffect(() => {
    if (registries !== undefined && registries.length > 0) {
      setSelectedRegistry(registries[0]);
    }
  }, [registries]);

  const { data, error: registryError, isPlaceholderData, isFetching } = useQuery({
    queryKey: ['registry', filters, activeFilter, refNames, sortAscending, rowsPerPage, page],
    queryFn: () => fetchRegistryEntries({
      registryName: selectedRegistry!,
      filters,
      tab: activeFilter,
      limit: rowsPerPage,
      pageParam: refNames[refNames.length - 1],
      sortAscending,
    }),
    enabled: selectedRegistry !== undefined,
    placeholderData: keepPreviousData
  });

  const registryEntries = data?.items;
  const hasMore = data?.hasMore ?? false;

  const count = pagedTableCount(data, hasMore, page, rowsPerPage);

  useResetPaginationOnChange(() => {
    setRefNames([]);
    setPage(0);
  }, filters, activeFilter);

  if (registriesError !== null || registryError !== null) {
    return <Alert sx={{ margin: '30px' }} severity="error" variant="filled">{registriesError?.message ?? registryError?.message}</Alert>
  }

  const handleChangePage = (
    _event: React.MouseEvent<HTMLButtonElement> | null,
    newPage: number
  ) => {
    if (newPage === 0) {
      setRefNames([]);
    } else if (newPage > page) {
      if (registryEntries !== undefined && !isPlaceholderData && registryEntries.length > 0) {
        const refEntriesCopy = [...refNames];
        refEntriesCopy.push(registryEntries[registryEntries.length - 1].name);
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
              {t('registries')}
            </Typography>
            <TextField
              size="small"
              color="secondary"
              slotProps={{
                input: {
                  sx: {
                    color: theme => theme.palette.text.secondary,
                    fontWeight: 500,
                    height: '28px',
                    fontSize: '15px'
                  }
                }
              }}
              select={registries !== undefined && registries?.length > 0}
              value={selectedRegistry ?? ''}
              onChange={event => setSelectedRegistry(event.target.value)}
            >
              {registries?.map(registry =>
                <MenuItem key={registry} value={registry}>{registry}</MenuItem>
              )}
            </TextField>
            <ToggleButtonGroup sx={{ height: '30px' }} size="small" exclusive onChange={(_event, value) => {
              if (value !== null) {
                setActiveFilter(value);
              }
            }} value={activeFilter}>
              <ToggleButton color="primary" value="any" sx={{ width: '120px' }}>{t('all')}</ToggleButton>
              <ToggleButton color="primary" value="active" sx={{ width: '120px' }}>{t('active')}</ToggleButton>
              <ToggleButton color="primary" value="inactive" sx={{ width: '120px' }}>{t('inactive')}</ToggleButton>
            </ToggleButtonGroup>
            <Box sx={{ flexGrow: 1, display: 'flex', justifyContent: 'right', gap: '10px' }}>
              <Button
                size="small"
                variant="outlined"
                startIcon={<PersonSearchIcon />}
                sx={{ borderRadius: '20px', minWidth: '120px' }}
                onClick={() => setResolveVerifierDialogOpen(true)}
              >
                {t('resolve')}
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
                    label: t('name'),
                    name: '.name',
                    type: 'string'
                  },
                  {
                    label: t('id'),
                    name: '.id',
                    type: 'string',
                    isHexValue: true
                  },
                  {
                    label: t('owner'),
                    name: '$owner',
                    type: 'string',
                    isHexValue: true
                  }
                ]}
                filters={filters}
                setFilters={setFilters}
              />
            </Box>
          </Collapse>
          {selectedRegistry !== undefined && registryEntries !== undefined && registryEntries.length > 0 &&
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
                            setRefNames([]);
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
                        {t('owner')}
                      </TableCell>
                      <TableCell
                        width={'100%'}
                        sx={{
                          backgroundColor: (theme) => theme.palette.background.paper,
                          whiteSpace: 'nowrap'
                        }}
                      >
                        {t('status')}
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
                    {registryEntries.map(registryEntry =>
                      <TableRow key={registryEntry.id}>
                        <TableCell>
                          {registryEntry.name}
                        </TableCell>
                        <TableCell sx={{ paddingTop: '8px', paddingBottom: '8px' }}>
                          <Hash Icon={<Tag size="18px" />} hideTitle title={t('id')} hash={registryEntry.id} />
                        </TableCell>
                        <TableCell sx={{ paddingTop: '8px', paddingBottom: '8px' }}>
                          <Hash Icon={<Captions size="18px" />} hideTitle title={t('owner')} hash={registryEntry.properties.$owner} />
                        </TableCell>
                        <TableCell>
                          {t(registryEntry.active !== false ? 'active' : 'inactive')}
                        </TableCell>
                        <TableCell align="right" sx={{ paddingTop: '8px', paddingBottom: '8px' }}>
                          <Tooltip title={t('open')} arrow>
                            <IconButton
                              onClick={mouseEvent => customNavigate(AppRouteFactory.getPath('RegistryEntry', { registry: selectedRegistry, id: registryEntry.id }), mouseEvent, navigate)}>
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
          {registryEntries !== undefined && registryEntries.length === 0 &&
            <Box sx={{ marginTop: '20px', textAlign: 'center', color: theme => theme.palette.text.secondary }}>
              <InfoOutlinedIcon sx={{ fontSize: '50px' }} />
              <Typography>{t('noRegistryEntries')}</Typography>
            </Box>
          }
        </Box>
      </Fade>
      {resolveVerifierDialogOpen && (
        <ResolveVerifierDialog
          onClose={() => setResolveVerifierDialogOpen(false)}
        />
      )}
    </>
  );
};
