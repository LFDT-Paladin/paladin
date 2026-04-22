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

import {
  Alert,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TablePagination,
  TableRow,
  TableSortLabel,
  Typography,
  useTheme,
} from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { querySmartContractsByDomain } from '../queries/domains';
import { getAltModeScrollBarStyle } from '../themes/default';
import { DomainButtons } from './DomainButtons';
import { Hash } from './Hash';
import { IDomainContract } from '../interfaces';
import { Timestamp } from './Timestamp';
import { Dispatch, SetStateAction, useEffect, useState } from 'react';

type Props = {
  domainAddress: string
  sortAscending: boolean
  setSortAscending: Dispatch<SetStateAction<boolean>>
  page: number
  setPage: Dispatch<SetStateAction<number>>
  rowsPerPage: number
  setRowsPerPage: Dispatch<SetStateAction<number>>
  refTimestamps: string[]
  setRefTimestamps: Dispatch<SetStateAction<string[]>>
};

export const SmartContractsTable: React.FC<Props> = ({
  domainAddress,
  sortAscending,
  setSortAscending,
  page,
  setPage,
  rowsPerPage,
  setRowsPerPage,
  refTimestamps,
  setRefTimestamps
}) => {

  const [count, setCount] = useState(-1);
  const { t } = useTranslation();
  const theme = useTheme();

  const {
    data: contracts,
    error,
  } = useQuery({
    queryKey: ['contracts', domainAddress, sortAscending, page, rowsPerPage, refTimestamps],
    queryFn: () => querySmartContractsByDomain(domainAddress, sortAscending, rowsPerPage, refTimestamps[refTimestamps.length - 1]),
  });

  if (error) {
    return (
      <Alert sx={{ margin: '30px' }} severity="error" variant="filled">
        {error.message}
      </Alert>
    );
  }

  useEffect(() => {
    if (contracts !== undefined && count === -1) {
      if (contracts.length < rowsPerPage) {
        setCount(rowsPerPage * page + contracts.length);
      }
    }
  }, [contracts, rowsPerPage, page]);

  const handleChangePage = (
    _event: React.MouseEvent<HTMLButtonElement> | null,
    newPage: number
  ) => {
    if (newPage === 0) {
      setRefTimestamps([]);
    } else if (newPage > page) {
      if (contracts !== undefined) {
        const refEntriesCopy = [...refTimestamps];
        refEntriesCopy.push(contracts[contracts.length - 1].deployTransaction.created);
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
    <TableContainer
      component={Paper}
      sx={{
        ...getAltModeScrollBarStyle(theme.palette.mode),
      }}
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
                active={true}
                direction={sortAscending ? 'asc' : 'desc'}
                onClick={() => 
                  {
                    setSortAscending(!sortAscending);
                    setRefTimestamps([]);
                    setPage(0);
                  }}
              >
                {t('deployed')}
              </TableSortLabel>
            </TableCell>
            <TableCell
              sx={{
                backgroundColor: (theme) => theme.palette.background.paper,
              }}
            >
              {t('contractAddress')}
            </TableCell>
            <TableCell
              sx={{
                backgroundColor: (theme) => theme.palette.background.paper,
              }}
            >
              {t('actions')}
            </TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {contracts?.map((contract: IDomainContract) => (
            <TableRow key={contract.address} sx={{ height: '70px' }}>
              <TableCell>
                <Timestamp timestamp={contract.deployTransaction.created} />
              </TableCell>
              <TableCell>
                <Hash title={t('address')} hash={contract.address} />
              </TableCell>
              <TableCell>
                <DomainButtons
                  domainName={contract.domainName}
                  contractAddress={contract.address}
                />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      {contracts?.length === 0 ?
        <Typography color="textSecondary" align="center" variant="h6" sx={{ marginTop: '40px' }}>
          {t('noSmartContracts')}
        </Typography>
        :
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
        />}
    </TableContainer>
  );
};
