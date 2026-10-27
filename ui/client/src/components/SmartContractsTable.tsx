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

import {
  Alert,
  Box,
  IconButton,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TablePagination,
  TableRow,
  TableSortLabel,
  Tooltip,
  Typography
} from '@mui/material';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { querySmartContractsByDomain, buildDomainContractPagingReference } from '../queries/domains';
import { DomainButtons } from './DomainButtons';
import { Hash } from './Hash';
import { IDomainContract, IFilter, ISortPagingReference } from '../interfaces';
import { Timestamp } from './Timestamp';
import { Dispatch, SetStateAction, useEffect } from 'react';
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import { useNavigate } from 'react-router-dom';
import { customNavigate } from '../utils';
import { Captions } from 'lucide-react';
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import { pagedTableCount, useResetPaginationOnChange } from '../hooks/pagination';
import { AppRouteFactory } from '../routes';

type Props = {
  domainAddress: string
  sortAscending: boolean
  setSortAscending: Dispatch<SetStateAction<boolean>>
  page: number
  setPage: Dispatch<SetStateAction<number>>
  rowsPerPage: number
  setRowsPerPage: Dispatch<SetStateAction<number>>
  refEntries: ISortPagingReference[]
  setRefEntries: Dispatch<SetStateAction<ISortPagingReference[]>>
  selectedDomain?: string,
  filters: IFilter[]
};

export const SmartContractsTable: React.FC<Props> = ({
  domainAddress,
  sortAscending,
  setSortAscending,
  page,
  setPage,
  rowsPerPage,
  setRowsPerPage,
  refEntries,
  setRefEntries,
  selectedDomain,
  filters
}) => {

  const { t } = useTranslation();
  const navigate = useNavigate();

  const {
    data,
    error,
    isPlaceholderData,
    isFetching
  } = useQuery({
    queryKey: ['contracts', domainAddress, sortAscending, page, rowsPerPage, filters, refEntries],
    queryFn: () => querySmartContractsByDomain({
      domainAddress,
      sortAscending,
      limit: rowsPerPage,
      filters,
      pageRef: refEntries[refEntries.length - 1],
    }),
    placeholderData: keepPreviousData
  });

  const contracts = data?.items;
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
      if (contracts !== undefined && !isPlaceholderData && contracts.length > 0) {
        const refEntriesCopy = [...refEntries];
        refEntriesCopy.push(buildDomainContractPagingReference(contracts[contracts.length - 1]));
        setRefEntries(refEntriesCopy);
      }
    } else {
      const refEntriesCopy = [...refEntries];
      refEntriesCopy.pop();
      setRefEntries(refEntriesCopy);
    }
    setPage(newPage);
  };

  useEffect(() => {
    if (count !== -1 && page !== 0 && page * rowsPerPage === count) {
      handleChangePage(null, page - 1);
    }
  }, [count, rowsPerPage, page]);

  const handleChangeRowsPerPage = (
    event: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>
  ) => {
    const value = parseInt(event.target.value, 10);
    setRowsPerPage(value);
    setRefEntries([]);
    setPage(0);
  };

  if (error) {
    return (
      <Alert sx={{ margin: '30px' }} severity="error" variant="filled">
        {error.message}
      </Alert>
    );
  }

  return (
    <>
      {contracts !== undefined && contracts.length > 0 &&
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
                      {t('deployed')}
                    </TableSortLabel>
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
                  {selectedDomain === 'noto' &&
                    <TableCell
                      width={1}
                      sx={{
                        backgroundColor: (theme) => theme.palette.background.paper,
                        whiteSpace: 'nowrap'
                      }}
                    >
                      {t('name')}
                    </TableCell>}
                  {selectedDomain === 'noto' &&
                    <TableCell
                      width={1}
                      sx={{
                        backgroundColor: (theme) => theme.palette.background.paper,
                        whiteSpace: 'nowrap'
                      }}
                    >
                      {t('symbol')}
                    </TableCell>}
                  {selectedDomain === 'noto' &&
                    <TableCell
                      width={1}
                      sx={{
                        backgroundColor: (theme) => theme.palette.background.paper,
                        whiteSpace: 'nowrap'
                      }}
                    >
                      {t('isNotary')}
                    </TableCell>}
                  {selectedDomain === 'zeto' &&
                    <TableCell
                      width={1}
                      sx={{
                        backgroundColor: (theme) => theme.palette.background.paper,
                        whiteSpace: 'nowrap'
                      }}
                    >
                      {t('tokenName')}
                    </TableCell>}
                  <TableCell
                    sx={{
                      backgroundColor: (theme) => theme.palette.background.paper,
                      whiteSpace: 'nowrap'
                    }}
                  >
                    {t('actions')}
                  </TableCell>
                  <TableCell
                    sx={{
                      backgroundColor: (theme) => theme.palette.background.paper,
                      whiteSpace: 'nowrap'
                    }}
                  />
                </TableRow>
              </TableHead>
              <TableBody>
                {contracts?.map((contract: IDomainContract) => (
                  <TableRow key={contract.address} >
                    <TableCell sx={{ paddingTop: '8px', paddingBottom: '8px' }}>
                      <Timestamp timestamp={contract.created} />
                    </TableCell>
                    <TableCell sx={{ paddingTop: '8px', paddingBottom: '8px' }}>
                      <Hash Icon={<Captions size="18px" />} hideTitle title={t('address')} hash={contract.address} />
                    </TableCell>
                    {selectedDomain === 'noto' && 'name' in contract.config.contractConfig &&
                      <TableCell>
                        {contract.config.contractConfig.name.length > 0 ? contract.config.contractConfig.name : '--'}
                      </TableCell>}
                    {selectedDomain === 'noto' && 'symbol' in contract.config.contractConfig &&
                      <TableCell>
                        {contract.config.contractConfig.symbol.length > 0 ? contract.config.contractConfig.symbol : '--'}
                      </TableCell>}
                    {selectedDomain === 'noto' && 'isNotary' in contract.config.contractConfig &&
                      <TableCell>
                        {t(contract.config.contractConfig.isNotary ? 'yes' : 'no')}
                      </TableCell>}
                    {selectedDomain === 'zeto' && 'tokenName' in contract.config.contractConfig &&
                      <TableCell>
                        {contract.config.contractConfig.tokenName.length > 0 ? contract.config.contractConfig.tokenName : '--'}
                      </TableCell>}
                    <TableCell sx={{ paddingTop: '8px', paddingBottom: '8px' }}>
                      <DomainButtons
                        domainName={contract.domainName}
                        contractAddress={contract.address}
                      />
                    </TableCell>
                    <TableCell align="right" sx={{ paddingTop: '8px', paddingBottom: '8px' }}>
                      <Tooltip title={t('open')} arrow>
                        <IconButton
                          onClick={mouseEvent => customNavigate(AppRouteFactory.getPath('DomainContract', { address: contract.address }), mouseEvent, navigate)}>
                          <OpenInNewIcon color="secondary" fontSize="medium" />
                        </IconButton>
                      </Tooltip>
                    </TableCell>
                  </TableRow>
                ))}
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
      {contracts !== undefined && contracts.length === 0 &&
        <Box sx={{ marginTop: '20px', textAlign: 'center', color: theme => theme.palette.text.secondary }}>
          <InfoOutlinedIcon sx={{ fontSize: '50px' }} />
          <Typography>
            {t('noSmartContracts')}
          </Typography>
        </Box>}
    </>
  );
};