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

import { Alert, Box, Breadcrumbs, Button, Collapse, Fade, IconButton, Link, Paper, Table, TableBody, TableCell, TableContainer, TableHead, TablePagination, TableRow, TableSortLabel, ToggleButton, ToggleButtonGroup, Tooltip, Typography } from "@mui/material";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { useApplicationContext } from "../contexts/ApplicationContext";
import { fetchKeys } from "../queries/keys";
import { Hash } from "../components/Hash";
import FolderOpenIcon from '@mui/icons-material/FolderOpen';
import NavigateNextIcon from '@mui/icons-material/NavigateNext';
import { IKeyEntry, IVerifier } from "../interfaces";
import { useSearchParams } from "react-router-dom";
import { Captions, Signature } from "lucide-react";
import SearchIcon from '@mui/icons-material/Search';
import { ReverseKeyLookupDialog } from "../dialogs/ReverseKeyLookup";
import RemoveIcon from '@mui/icons-material/Remove';
import { VerifiersDialog } from "../dialogs/Verifiers";
import { useTranslation } from "react-i18next";
import { Filters } from "../components/Filters";
import { FiltersButton } from "../components/FiltersButton";
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import { pagedTableCount, useResetPaginationOnChange } from "../hooks/pagination";

export const Keys: React.FC = () => {
  const { keys: keysViewState } = useApplicationContext();
  const {
    page,
    setPage,
    rowsPerPage,
    setRowsPerPage,
    mode,
    setMode,
    filters,
    setFilters,
    sortAscending,
    setSortAscending,
    sortByPathFirst,
    setSortByPathFirst,
    filtersVisible,
    setFiltersVisible,
  } = keysViewState;

  const [searchParams, setSearchParams] = useSearchParams();
  const [refEntries, setRefEntries] = useState<IKeyEntry[]>([]);
  const [parent, setParent] = useState(searchParams.get('path') ?? '');
  const [reverseLookupDialogOpen, setReverseLookupDialogOpen] = useState(false);
  const [selectedVerifiers, setSelectedVerifiers] = useState<IVerifier[]>();
  const [verifiersDialogOpen, setVerifiersDialogOpen] = useState(false);
  const { t } = useTranslation();

  useEffect(() => {
    setParent(searchParams.get('path') ?? '');
  }, [searchParams]);

  const { data, error, isPlaceholderData, isFetching } = useQuery({
    queryKey: ["keys", parent, sortByPathFirst, sortAscending, refEntries, rowsPerPage, filters, mode],
    queryFn: () => fetchKeys(mode === 'explorer' ? parent : undefined, rowsPerPage, sortByPathFirst, sortAscending ? 'asc' : 'desc', filters, refEntries[refEntries.length - 1]),
    placeholderData: keepPreviousData
  });

  const keys = data?.items;
  const hasMore = data?.hasMore ?? false;

  const count = pagedTableCount(data, hasMore, page, rowsPerPage);

  useEffect(() => {
    if (count !== -1 && (page !== 0 && page * rowsPerPage === count)) {
      handleChangePage(null, page - 1);
    }
  }, [count, rowsPerPage, page]);

  useResetPaginationOnChange(() => {
    setPage(0);
    setRefEntries([]);
  }, parent);

  useEffect(() => {
    let value: any = {};
    if (parent !== '') {
      value.path = parent;
    }
    setSearchParams(value);
  }, [parent, page]);

  useResetPaginationOnChange(() => {
    setPage(0);
    setRefEntries([]);
  }, filters);

  useResetPaginationOnChange(() => {
    if (mode === 'list') {
      setParent('');
    }
    setPage(0);
    setRefEntries([]);
  }, mode);

  if (error) {
    return <Alert sx={{ margin: '30px' }} severity="error" variant="filled">{error.message}</Alert>
  }

  let breadcrumbContent: JSX.Element[] = [];
  if (parent !== '') {
    const segments = parent.split('.');
    let segmentStack: string[] = [];
    for (const segment of segments) {
      segmentStack.push(segment);
      const target = segmentStack.join('.');
      breadcrumbContent.push(
        <Link underline="none"
          key={segment}
          href=""
          onClick={event => {
            event.preventDefault();
            setParent(target);
          }}>
          {segment === '' ? t('root') : segment}
        </Link>
      )
    }
  }

  const getEthAddress = (key: IKeyEntry) => {
    const entry = key.verifiers?.find(entry => entry.type === 'eth_address');
    if (entry !== undefined) {
      return <Hash
        Icon={<Captions size="18px" />}
        title={entry.algorithm}
        hash={entry.verifier}
        hideTitle />
    }
    return <RemoveIcon color="disabled" />;
  };

  const getOtherVerifiers = (key: IKeyEntry) => {
    if (key.verifiers !== null) {
      const entries = key.verifiers.filter(entry => entry.type !== 'eth_address');
      if (entries.length === 1) {
        return <Hash
          Icon={<Signature size="18px" />}
          title={entries[0].algorithm}
          hash={entries[0].verifier}
          hideTitle />
      } else if (entries.length > 1) {
        return (
          <Button
            variant="contained"
            disableElevation
            color="primary"
            size="small"
            sx={{ minWidth: '110px', paddingTop: 0, paddingBottom: 0, fontWeight: '400', whiteSpace: 'nowrap' }}
            onClick={() => { setSelectedVerifiers(entries); setVerifiersDialogOpen(true) }}
          >
            {t('manyN', { n: entries.length })}
          </Button>
        );
      }
    }
    return <RemoveIcon color="disabled" />;
  };

  const removeParentFromPath = (path: string) => {
    let index = parent.length;
    if (index > 0) {
      index++;
    }
    return path.substring(index);
  }

  const handleChangePage = (
    _event: React.MouseEvent<HTMLButtonElement> | null,
    newPage: number
  ) => {
    if (newPage === 0) {
      setRefEntries([]);
    } else if (newPage > page) {
      if (keys !== undefined && !isPlaceholderData && keys.length > 0) {
        const refEntriesCopy = [...refEntries];
        refEntriesCopy.push(keys[keys.length - 1]);
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

  const headerDivider = <Box sx={{
    height: '30px',
    width: '1px',
    border: theme => `solid 1px ${theme.palette.divider}`,
    position: 'absolute',
    top: '14px',
    left: '2px'
  }} />;

  return (
    <>
      <Fade timeout={300} in={true}>
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
              {t("localKeys")}
            </Typography>
            <ToggleButtonGroup exclusive
              size="small"
              sx={{ height: '30px' }}
              onChange={(_event, value) => {
                if (value !== null) {
                  setMode(value);
                }
              }}
              value={mode}>
              <ToggleButton color="primary" value="list" sx={{ width: '120px' }}>
                {t('listView')}
              </ToggleButton>
              <ToggleButton color="primary" value="explorer" sx={{ width: '120px' }}>
                {t('explorerView')}
              </ToggleButton>
            </ToggleButtonGroup>
            {mode === 'explorer' &&
              <Breadcrumbs
                separator={<NavigateNextIcon fontSize="small" />}
                sx={{ marginLeft: '10px' }}>
                <Link underline="none"
                  href=""
                  onClick={event => { event.preventDefault(); setParent('') }}>
                  {t('root')}
                </Link>
                {breadcrumbContent}
              </Breadcrumbs>}
            <Box sx={{ flexGrow: 1, display: 'flex', justifyContent: 'right', gap: '10px' }}>
              <Button
                sx={{ borderRadius: '20px', minWidth: '120px' }}
                size="small"
                variant="outlined"
                startIcon={<SearchIcon />}
                onClick={() => setReverseLookupDialogOpen(true)}
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
                    label: t('path'),
                    name: 'path',
                    type: 'string'
                  },
                  {
                    label: t('index'),
                    name: 'index',
                    type: 'number'
                  },
                  {
                    label: t('wallet'),
                    name: 'wallet',
                    type: 'string'
                  },
                  {
                    label: t('handle'),
                    name: 'keyHandle',
                    type: 'string'
                  },
                  {
                    label: t('isFolder'),
                    name: 'hasChildren',
                    type: 'boolean'
                  },
                  {
                    label: t('isKey'),
                    name: 'isKey',
                    type: 'boolean'
                  }
                ]}
                filters={filters}
                setFilters={setFilters}
              />
            </Box>
          </Collapse>
          {keys !== undefined && keys.length > 0 &&
            <Paper>
              <TableContainer>
                <Table stickyHeader>
                  <TableHead>
                    <TableRow>
                      {mode === 'explorer' &&
                        <TableCell width={1} sx={{ minWidth: '70px', backgroundColor: theme => theme.palette.background.paper }} />
                      }
                      <TableCell sx={{ backgroundColor: theme => theme.palette.background.paper, maxWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        <TableSortLabel
                          active={sortByPathFirst}
                          direction={sortAscending ? 'asc' : 'desc'}
                          onClick={() => {
                            if (sortByPathFirst) {
                              setSortAscending(!sortAscending)
                            } else {
                              setSortByPathFirst(true);
                            }
                            setPage(0);
                            setRefEntries([]);
                          }}
                        >
                          {t(mode === 'explorer' ? 'pathSegment' : 'path')}
                        </TableSortLabel>
                        {mode === 'explorer' && headerDivider}
                      </TableCell>
                      <TableCell width={1} sx={{ backgroundColor: theme => theme.palette.background.paper }}>
                        <TableSortLabel
                          active={!sortByPathFirst}
                          direction={sortAscending ? 'asc' : 'desc'}
                          onClick={() => {
                            if (!sortByPathFirst) {
                              setSortAscending(!sortAscending)
                            } else {
                              setSortByPathFirst(false);
                            }
                            setPage(0);
                            setRefEntries([]);
                          }}
                        >
                          {t('index')}
                        </TableSortLabel>
                        {headerDivider}
                      </TableCell>
                      <TableCell sx={{ minWidth: '160px', backgroundColor: theme => theme.palette.background.paper }} width={1} >{t('address')}{headerDivider}</TableCell>
                      <TableCell sx={{ minWidth: '160px', backgroundColor: theme => theme.palette.background.paper, whiteSpace: 'nowrap' }} width={1} >{t('otherVerifiers')}{headerDivider}</TableCell>
                      <TableCell sx={{ minWidth: '160px', backgroundColor: theme => theme.palette.background.paper }} width={1}>{t('wallet')}{headerDivider}</TableCell>
                      <TableCell sx={{ minWidth: '160px', backgroundColor: theme => theme.palette.background.paper }} width={1} >{t('handle')}{headerDivider}</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {keys?.map(key =>
                      <TableRow key={`${key.path}${key.index}`}>
                        {mode === 'explorer' &&
                          <TableCell sx={{ paddingTop: '8px', paddingBottom: '8px' }}>{key.hasChildren &&
                            <Tooltip arrow title={t('openFolder')}>
                              <IconButton onClick={() => { setParent(key.path) }}>
                                <FolderOpenIcon fontSize="small" />
                              </IconButton>
                            </Tooltip>
                          }</TableCell>}
                        <TableCell sx={{ minWidth: '200px', maxWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{mode === 'explorer' ? removeParentFromPath(key.path) : key.path}</TableCell>
                        <TableCell>{key.index}</TableCell>
                        <TableCell sx={{ paddingTop: '8px', paddingBottom: '8px' }}>
                          {getEthAddress(key)}
                        </TableCell>
                        <TableCell sx={{ paddingTop: '8px', paddingBottom: '8px' }}>
                          {getOtherVerifiers(key)}
                        </TableCell>
                        <TableCell sx={{ whiteSpace: 'nowrap', padding: '8px' }}>{key.wallet.length > 0 ? key.wallet : <RemoveIcon color="disabled" />}</TableCell>
                        <TableCell sx={{ paddingTop: '8px', paddingBottom: '8px' }}>{key.keyHandle.length > 0 ?
                          <Hash hash={key.keyHandle} title={t('handle')} hideTitle secondary />
                          : <RemoveIcon color="disabled" />}</TableCell>
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
          {keys !== undefined && keys.length === 0 &&
            <Box sx={{ marginTop: '20px', textAlign: 'center', color: theme => theme.palette.text.secondary }}>
              <InfoOutlinedIcon sx={{ fontSize: '50px' }} />
              <Typography>{t('keysEmptyState')}</Typography>
            </Box>
          }
        </Box>
      </Fade>
      <ReverseKeyLookupDialog
        dialogOpen={reverseLookupDialogOpen}
        setDialogOpen={setReverseLookupDialogOpen}
        mode={mode}
        setParent={setParent}
        setFilters={setFilters}
      />
      {selectedVerifiers &&
        <VerifiersDialog
          dialogOpen={verifiersDialogOpen}
          setDialogOpen={setVerifiersDialogOpen}
          verifiers={selectedVerifiers}
        />}

    </>
  );
}
