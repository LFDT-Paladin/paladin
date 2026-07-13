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

import { Alert, Box, Button, Collapse, Fade, IconButton, MenuItem, Paper, Table, TableBody, TableCell, TableContainer, TableHead, TablePagination, TableRow, TableSortLabel, TextField, Tooltip, Typography, useTheme } from "@mui/material";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { listDomains } from "../queries/domains";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { useApplicationContext } from "../contexts/ApplicationContext";
import { listSchemas, queryStates, buildStatePagingReference } from "../queries/states";
import { Timestamp } from "../components/Timestamp";
import { Captions, Tag } from "lucide-react";
import { customNavigate } from "../utils";
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import { Hash } from "../components/Hash";
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import { IFilterField, ISchemaComponent, IState } from "../interfaces";
import { Filters } from "../components/Filters";
import { StateActions } from "../components/StateActions";
import { FiltersButton } from "../components/FiltersButton";
import { StateLookupDialog } from "../dialogs/StateLookup";
import SearchIcon from '@mui/icons-material/Search';
import { pagedTableCount, useResetPaginationOnChange } from "../hooks/pagination";

export const States: React.FC = () => {
  const { states: statesViewState, readOnly } = useApplicationContext();
  const {
    sortAscending,
    setSortAscending,
    refEntries,
    setRefEntries,
    page,
    setPage,
    rowsPerPage,
    setRowsPerPage,
    selectedDomain,
    setSelectedDomain,
    selectedSchemaId,
    setSelectedSchemaId,
    filters,
    setFilters,
    filtersVisible,
    setFiltersVisible,
  } = statesViewState;

  const [stateLookupDialogOpen, setStateLookupDialogOpen] = useState(false);
  const [sortBy, setSortBy] = useState('.created');
  const tableIndexedFieldsRef = useRef<ISchemaComponent[]>([]);
  const theme = useTheme();
  const navigate = useNavigate();
  const { t } = useTranslation();

  const { data: domains, error: domainsError } = useQuery({
    queryKey: ['domains'],
    queryFn: () => listDomains(),
  });

  const { data: schemas, error: schemasError } = useQuery({
    queryKey: ['schemas', selectedDomain],
    queryFn: () => listSchemas(selectedDomain!),
    enabled: selectedDomain !== undefined
  });

  const { data, error: statesError, isPlaceholderData, isFetching } = useQuery({
    queryKey: ['states', selectedDomain, selectedSchemaId, page, rowsPerPage, sortBy, sortAscending, filters, refEntries],
    queryFn: () => queryStates(selectedDomain!, selectedSchemaId!, rowsPerPage, sortBy, sortAscending, filters, refEntries[refEntries.length - 1]),
    enabled: selectedSchemaId !== undefined,
    placeholderData: keepPreviousData
  });

  const states = data?.items;
  const hasMore = data?.hasMore ?? false;

  const count = pagedTableCount(data, hasMore, page, rowsPerPage);

  useEffect(() => {
    if (selectedDomain === undefined && domains !== undefined && domains.length > 0) {
      setSelectedDomain(domains[0]);
    }
  }, [selectedDomain, domains]);

  useEffect(() => {
    if (selectedDomain !== undefined && schemas !== undefined && schemas.length > 0) {
      setSelectedSchemaId(schemas[0].id);
    }
  }, [selectedDomain, schemas]);

  useResetPaginationOnChange(() => {
    setRefEntries([]);
    setPage(0);
  }, filters);

  const selectedIndexedFields =
    schemas?.find(schema => schema.id === selectedSchemaId)
      ?.definition.components.filter(component => component.indexed) ?? [];

  const statesMatchSelection =
    states !== undefined &&
    (states.length === 0 || states[0].schema === selectedSchemaId);

  const tableIndexedFields = statesMatchSelection
    ? selectedIndexedFields
    : tableIndexedFieldsRef.current;

  if (statesMatchSelection) {
    tableIndexedFieldsRef.current = selectedIndexedFields;
  }

  if (domainsError || schemasError || statesError) {
    return <Alert sx={{ margin: '30px' }} severity="error" variant="filled">
      {domainsError?.message ?? schemasError?.message ?? statesError?.message}
    </Alert>
  }

  if (domains === undefined) {
    return <></>
  }

  const resetPagination = () => {
    setRefEntries([]);
    setPage(0);
  };

  const handleChangePage = (
    _event: React.MouseEvent<HTMLButtonElement> | null,
    newPage: number
  ) => {
    if (newPage === 0) {
      setRefEntries([]);
    } else if (newPage > page) {
      if (states !== undefined && !isPlaceholderData && states.length > 0) {
        const refEntriesCopy = [...refEntries];
        refEntriesCopy.push(buildStatePagingReference(states[states.length - 1], sortBy));
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
    resetPagination();
  };

  const getIndexedFieldContent = (state: IState, component: ISchemaComponent) => {
    const value = state.data[component.name];
    if (value === undefined || value === null) {
      return <>--</>;
    }
    if (component.type.startsWith('byte')) {
      return <Hash Icon={<Captions size="18px" />} hideTitle title={component.type} hash={value} />
    }
    switch (component.type) {
      case 'address':
        return <Hash Icon={<Captions size="18px" />} hideTitle title={t('address')} hash={value} />
      case 'bool': return value ? 'true' : 'false';
      default: return value;
    }
  };

  const filterFields: IFilterField[] = [
    {
      label: t('created'),
      name: '.created',
      type: 'timestamp',
      isNanoSeconds: true
    },
    {
      label: t('id'),
      name: '.id',
      type: 'string',
      isHexValue: true
    },
    {
      label: t('contractAddress'),
      name: 'contractAddress',
      type: 'string',
      isHexValue: true
    }
  ];

  const getFilterType = (field: ISchemaComponent) => {
    if (field.type.startsWith('int') || field.type.startsWith('uint')) {
      return 'number';
    } else if (field.type === 'bool') {
      return 'boolean';
    }
    return 'string';
  };

  selectedIndexedFields.map(indexedField => filterFields.push({
    label: `${indexedField.name}`,
    name: indexedField.name,
    type: getFilterType(indexedField),
    isCustom: true,
    isHexValue: indexedField.type === 'address' || indexedField.type.startsWith('bytes')
  }));

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
              {t("states")}
            </Typography>
            <Box sx={{
              display: 'flex',
              alignItems: 'center',
              gap: '5px'
            }}>
              <Typography
                color="secondary"
                variant="body2"
              >{t('domain')}</Typography>
              <TextField
                size="small"
                fullWidth
                select
                value={selectedDomain ?? ''}
                sx={{ minWidth: '120px' }}
                onChange={event => {
                  setSelectedSchemaId(undefined);
                  setSelectedDomain(event.target.value);
                }}
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
              >
                {domains.map(domain =>
                  <MenuItem key={domain} value={domain}>
                    {t(domain)}
                  </MenuItem>
                )}
              </TextField>
            </Box>
            {selectedDomain !== undefined &&
              <Box sx={{
                display: 'flex',
                alignItems: 'center',
                gap: '5px'
              }}>
                <Typography
                  color="secondary"
                  variant="body2"
                >{t('schema')}</Typography>
                <TextField
                  sx={{ minWidth: '120px' }}

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
                  size="small"
                  fullWidth
                  select={schemas !== undefined}
                  disabled={schemas === undefined}
                  value={selectedSchemaId ?? ''}
                  onChange={event => {
                    resetPagination();
                    setSelectedSchemaId(event.target.value);
                    setFilters([]);
                  }}
                >
                  {schemas?.map(schema =>
                    <MenuItem key={schema.id} value={schema.id}>
                      <Box sx={{
                        display: 'flex',
                        gap: '10px'
                      }}>
                        <Typography>{schema.definition.name.length > 0 ? schema.definition.name : '--'}</Typography>
                        <Typography color="primary">{schema.labels.join(', ')}</Typography>
                      </Box>
                    </MenuItem>
                  )}
                </TextField>
              </Box>}
            {states !== undefined &&
              <>
                <Box sx={{ flexGrow: 1, display: 'flex', justifyContent: 'right', gap: '10px' }}>
                  <Button
                    sx={{ borderRadius: '20px', minWidth: '120px' }}
                    size="small"
                    variant="outlined"
                    startIcon={<SearchIcon />}
                    onClick={() => setStateLookupDialogOpen(true)}
                  >
                    {t('lookup')}
                  </Button>
                  <FiltersButton
                    filtersVisible={filtersVisible}
                    setFiltersVisible={setFiltersVisible}
                  />
                </Box>
              </>
            }
          </Box>
          <Collapse in={filtersVisible}>
            <Box sx={{ marginBottom: '20px' }}>
              <Filters
                filterFields={filterFields}
                filters={filters}
                setFilters={setFilters}
              />
            </Box>
          </Collapse>
          {states !== undefined && states.length > 0 &&
            <Paper>
              <TableContainer
                component={Paper}
              >
                <Table stickyHeader>
                  <TableHead>
                    <TableRow>
                      <TableCell
                        width={1}
                        sx={{
                          backgroundColor: (theme) => theme.palette.background.paper
                        }}>
                        <TableSortLabel
                          active={sortBy === '.created'}
                          direction={sortAscending ? 'asc' : 'desc'}
                          onClick={() => {
                            if (sortBy === '.created') {
                              setSortAscending(!sortAscending);
                            } else {
                              setSortBy('.created');
                            }
                            resetPagination();
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
                        {t('id')}
                      </TableCell>
                      <TableCell
                        width={1}
                        sx={{
                          backgroundColor: (theme) => theme.palette.background.paper,
                          whiteSpace: 'nowrap'
                        }}
                      >
                        {t('contractAddress')}
                      </TableCell>
                      {tableIndexedFields.map(field =>
                        <TableCell
                          key={field.name}
                          width={1}
                          sx={{
                            backgroundColor: (theme) => theme.palette.background.paper,
                            whiteSpace: 'nowrap'
                          }}>
                          <TableSortLabel
                            active={sortBy === field.name}
                            direction={sortAscending ? 'asc' : 'desc'}
                            onClick={() => {
                              if (sortBy === field.name) {
                                setSortAscending(!sortAscending);
                              } else {
                                setSortBy(field.name)
                              }
                              resetPagination();
                            }}
                          >
                            <span style={{ color: theme.palette.primary.main }}>{field.name}</span>
                          </TableSortLabel>
                        </TableCell>
                      )}
                      <TableCell
                        width={1}
                        sx={{
                          backgroundColor: (theme) => theme.palette.background.paper,
                          whiteSpace: 'nowrap'
                        }}
                      >
                        {readOnly ? '' : t('actions')}
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
                    {states.map(state =>
                      <TableRow key={state.id}>
                        <TableCell sx={{ paddingTop: '8px', paddingBottom: '8px' }}>
                          <Timestamp timestamp={state.created} />
                        </TableCell>
                        <TableCell sx={{ paddingTop: '8px', paddingBottom: '8px' }}>
                          <Hash Icon={<Tag size="18px" />} hideTitle title={t('id')} hash={state.id} />
                        </TableCell>
                        <TableCell sx={{ paddingTop: '8px', paddingBottom: '8px' }}>
                          {state.contractAddress !== null ?
                            <Hash Icon={<Captions size="18px" />} hideTitle title={t('address')} hash={state.contractAddress} />
                            :
                            <>--</>}
                        </TableCell>
                        {tableIndexedFields.map(field =>
                          <TableCell key={field.name}>
                            {getIndexedFieldContent(state, field)}
                          </TableCell>
                        )}
                        <TableCell sx={{ paddingTop: '8px', paddingBottom: '8px' }}>
                          {!readOnly &&
                            <StateActions state={state} />}
                        </TableCell>
                        <TableCell align="right" sx={{ paddingTop: '8px', paddingBottom: '8px' }}>
                          <Tooltip title={t('open')} arrow>
                            <IconButton
                              onClick={mouseEvent => customNavigate(`/ui/states/${state.domain}/${state.schema}/${state.id}`, mouseEvent, navigate)}>
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
          {statesMatchSelection && states !== undefined && states.length === 0 &&
            <Box sx={{ marginTop: '20px', textAlign: 'center', color: theme => theme.palette.text.secondary }}>
              <InfoOutlinedIcon sx={{ fontSize: '50px' }} />
              <Typography>{t('statesEmptyState')}</Typography>
            </Box>
          }
        </Box>
      </Fade>
      {selectedDomain !== undefined && selectedSchemaId !== undefined &&
        <StateLookupDialog
          domain={selectedDomain}
          schemaId={selectedSchemaId}
          dialogOpen={stateLookupDialogOpen}
          setDialogOpen={setStateLookupDialogOpen}
        />}
    </>
  );

}