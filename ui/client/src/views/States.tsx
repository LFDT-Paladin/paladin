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

import { Alert, Box, Fade, Grid2, IconButton, MenuItem, Paper, Table, TableBody, TableCell, TableContainer, TableHead, TablePagination, TableRow, TableSortLabel, TextField, Tooltip, Typography } from "@mui/material";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { listDomains } from "../queries/domains";
import { useQuery } from "@tanstack/react-query";
import { Dispatch, SetStateAction, useEffect, useState } from "react";
import { listSchemas, queryStates } from "../queries/states";
import { Timestamp } from "../components/Timestamp";
import { Captions, Tag } from "lucide-react";
import { customNavigate } from "../utils";
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import { Hash } from "../components/Hash";
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import { IFilter, ISchemaComponent, IState } from "../interfaces";
import { Filters } from "../components/Filters";
import { StateActions } from "../components/StateActions";

type Props = {
  sortAscending: boolean
  setSortAscending: Dispatch<SetStateAction<boolean>>
  refTimestamps: string[]
  setRefTimestamps: Dispatch<SetStateAction<string[]>>
  page: number
  setPage: Dispatch<SetStateAction<number>>
  rowsPerPage: number
  setRowsPerPage: Dispatch<SetStateAction<number>>
  selectedDomain: string | undefined
  setSelectedDomain: Dispatch<SetStateAction<string | undefined>>
  selectedSchemaId: string | undefined
  setSelectedSchemaId: Dispatch<SetStateAction<string | undefined>>
  filters: IFilter[]
  setFilters: Dispatch<SetStateAction<IFilter[]>>
};

export const States: React.FC<Props> = ({
  sortAscending,
  setSortAscending,
  refTimestamps,
  setRefTimestamps,
  page,
  setPage,
  rowsPerPage,
  setRowsPerPage,
  selectedDomain,
  setSelectedDomain,
  selectedSchemaId,
  setSelectedSchemaId,
  filters,
  setFilters
}) => {

  const [count, setCount] = useState(-1);
  const [sortBy, setSortBy] = useState('.created');
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

  const { data: states, error: statesError } = useQuery({
    queryKey: ['states', selectedDomain, selectedSchemaId, page, rowsPerPage, sortBy, sortAscending, filters,],
    queryFn: () => queryStates(selectedDomain!, selectedSchemaId!, rowsPerPage, sortBy, sortAscending, filters, refTimestamps[refTimestamps.length - 1]),
    enabled: selectedSchemaId !== undefined
  });

  useEffect(() => {
    if (states !== undefined && count === -1) {
      if (states.length < rowsPerPage) {
        setCount(rowsPerPage * page + states.length);
      }
    }
  }, [states, rowsPerPage, page]);

  if (domains === undefined) {
    return <></>
  }

  if (domainsError || schemasError || statesError) {
    return <Alert sx={{ margin: '30px' }} severity="error" variant="filled">
      {domainsError?.message ?? schemasError?.message ?? statesError?.message}
    </Alert>
  }

  const handleChangePage = (
    _event: React.MouseEvent<HTMLButtonElement> | null,
    newPage: number
  ) => {
    if (newPage === 0) {
      setRefTimestamps([]);
    } else if (newPage > page) {
      if (states !== undefined) {
        const refEntriesCopy = [...refTimestamps];
        refEntriesCopy.push(states[states.length - 1].created);
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

  const indexedFields = schemas?.find(schema => schema.id === selectedSchemaId)?.definition.components.filter(component => component.indexed) ?? [];

  const getIndexedFieldContent = (state: IState, component: ISchemaComponent) => {
    switch (component.type) {
      case 'bytes32':
      case 'address': return <Hash Icon={<Captions size="18px" />} title={t('address')} hash={state.data[component.name]} />
      case 'bool': return state.data[component.name] ? 'true' : 'false';
      default: return state.data[component.name];
    }
  };

  const filterFields: any = [
    {
      label: t('id'),
      name: '.id',
      type: 'string'
    },
    {
      label: t('contractAddress'),
      name: 'contractAddress',
      type: 'string'
    }
  ];

  indexedFields.map(indexedField => filterFields.push({
    label: `[ ${indexedField.name} ]`,
    name: indexedField.name,
    type: indexedField.type === 'bool' ? 'boolean' : 'string'
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
          <Box sx={{ marginBottom: '20px' }}>
            <Typography align="center" variant="h5">
              {t("states")}
            </Typography>
          </Box>
          <Box sx={{
            display: 'flex',
            flexDirection: 'column',
            gap: '20px'
          }}>
            <Box
              sx={{
                backgroundColor: (theme) => theme.palette.background.paper,
                marginBottom: '20px',
                borderRadius: '4px',
                padding: '20px',
              }}
            >
              <Grid2 container spacing={2}>
                <Grid2 size={{ xs: 12, sm: 6 }}>
                  <TextField
                    fullWidth
                    label={t('domain')}
                    select
                    value={selectedDomain ?? ''}
                    onChange={event => {
                      setSelectedSchemaId(undefined);
                      setSelectedDomain(event.target.value);
                    }}
                  >
                    {domains.map(domain =>
                      <MenuItem key={domain} value={domain}>
                        {t(domain)}
                      </MenuItem>
                    )}
                  </TextField>
                </Grid2>
                <Grid2 size={{ xs: 12, sm: 6 }}>
                  <TextField
                    fullWidth
                    label={t('schema')}
                    select={schemas !== undefined}
                    disabled={schemas === undefined}
                    value={selectedSchemaId ?? ''}
                    onChange={event => {
                      setPage(0);
                      setCount(-1);
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
                </Grid2>
              </Grid2>
            </Box>
            {states !== undefined &&
              <Filters
                filterFields={filterFields}
                filters={filters}
                setFilters={setFilters}
              />
            }
            {states !== undefined && states.length > 0 &&
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
                      {indexedFields.map(field =>
                        <TableCell
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
                              setRefTimestamps([]);
                              setPage(0);
                            }}
                          >
                            [ {field.name} ]
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
                        {t('actions')}
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
                    {states.map(state =>
                      <TableRow key={state.id}>
                        <TableCell >
                          <Timestamp timestamp={state.created} />
                        </TableCell>
                        <TableCell>
                          <Hash Icon={<Tag size="18px" />} title={t('id')} hash={state.id} />
                        </TableCell>
                        <TableCell>
                          {state.contractAddress !== null ?
                            <Hash Icon={<Captions size="18px" />} title={t('address')} hash={state.contractAddress} />
                            :
                            <>--</>}
                        </TableCell>
                        {indexedFields.map(field =>
                          <TableCell key={field.name}>
                            {getIndexedFieldContent(state, field)}
                          </TableCell>
                        )}
                        <TableCell>
                          <StateActions state={state} />
                        </TableCell>
                        <TableCell align="right" sx={{ padding: '8px' }}>
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
            {states !== undefined && states.length === 0 &&
              <Box sx={{ marginTop: '60px', textAlign: 'center', color: theme => theme.palette.text.secondary }}>
                <InfoOutlinedIcon sx={{ fontSize: '50px' }} />
                <Typography>{t('statesEmptyState')}</Typography>
              </Box>
            }
          </Box>
        </Box>
      </Fade>
    </>
  );

}