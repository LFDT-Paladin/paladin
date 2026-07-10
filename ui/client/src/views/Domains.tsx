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
  Box,
  Button,
  Collapse,
  Fade,
  MenuItem,
  TextField,
  Typography
} from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { t } from 'i18next';
import { useEffect, useState } from 'react';
import { useApplicationContext } from '../contexts/ApplicationContext';
import { DomainDeploy } from '../components/DomainDeploy';
import { SmartContractsTable } from '../components/SmartContractsTable';
import { getDomainByName, listDomains } from '../queries/domains';
import SearchIcon from '@mui/icons-material/Search';
import { DomainContractLookupDialog } from '../dialogs/DomainContractLookup';
import { FiltersButton } from '../components/FiltersButton';
import { Filters } from '../components/Filters';

export const Domains: React.FC = () => {
  const { domains: domainsViewState, readOnly } = useApplicationContext();
  const {
    sortAscending,
    setSortAscending,
    page,
    setPage,
    rowsPerPage,
    setRowsPerPage,
    refEntries,
    setRefEntries,
    selectedDomain,
    setSelectedDomain,
    filters,
    setFilters,
    filtersVisible,
    setFiltersVisible,
  } = domainsViewState;

  const [lookupDomainContractDialogOpen, setLookupDomainContractDialogOpen] = useState(false);

  const {
    data: domains,
    error,
  } = useQuery({
    queryKey: ['domains'],
    queryFn: () => listDomains()
  });

  const { data: domain } = useQuery({
    queryKey: ['domain', selectedDomain],
    queryFn: () => getDomainByName(selectedDomain ?? ''),
    enabled: !!selectedDomain
  });

  useEffect(() => {
    if (selectedDomain === undefined && domains?.length) {
      setSelectedDomain(domains[0]);
    }
  }, [selectedDomain, domains]);

  useEffect(() => {
    setFilters([]);
  }, [selectedDomain]);

  if (error) {
    return (
      <Alert sx={{ margin: '30px' }} severity="error" variant="filled">
        {error.message}
      </Alert>
    );
  }

  return (
    <>
      <Fade timeout={600} in={true}>
        <Box
          sx={{
            padding: '20px',
            maxWidth: '1500px',
            marginLeft: 'auto',
            marginRight: 'auto',
          }}
        >
          <Box sx={{ display: 'flex', alignItems: 'center', gap: '20px', marginBottom: '20px', flexWrap: 'wrap' }}>
            <Typography variant="h5">
              {t('domainSmartContracts')}
            </Typography>
            <TextField
              sx={{ minWidth: '120px' }}
              size="small"
              color="secondary"
              slotProps={{
                input: {
                  sx: {
                    color: (theme) => theme.palette.text.secondary,
                    fontWeight: 500,
                    height: '28px',
                    fontSize: '15px'
                  }
                }
              }}
              select={domains !== undefined && domains.length > 0}
              value={selectedDomain ?? ''}
              onChange={(event) => setSelectedDomain(event.target.value)}
            >
              {domains?.map((domain) => (
                <MenuItem key={domain} value={domain}>
                  {t(domain)}
                </MenuItem>
              ))}
            </TextField>
            <Box sx={{ flexGrow: 1, display: 'flex', justifyContent: 'right', gap: '10px' }}>
              {!readOnly &&
                <DomainDeploy domainName={selectedDomain ?? ''} />}
              <Button
                sx={{ borderRadius: '20px', minWidth: '120px' }}
                size="small"
                variant="outlined"
                startIcon={<SearchIcon />}
                onClick={() => setLookupDomainContractDialogOpen(true)}
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
                    label: t('deployed'),
                    name: 'created',
                    type: 'timestamp',
                    isNanoSeconds: true
                  },
                  {
                    label: t('contractAddress'),
                    name: 'address',
                    type: 'string',
                    isHexValue: true
                  }
                ]}
                filters={filters}
                setFilters={setFilters}
              />
            </Box>
          </Collapse>
          {domain?.registryAddress && (
            <SmartContractsTable
              domainAddress={domain.registryAddress}
              sortAscending={sortAscending}
              setSortAscending={setSortAscending}
              page={page}
              setPage={setPage}
              rowsPerPage={rowsPerPage}
              setRowsPerPage={setRowsPerPage}
              refEntries={refEntries}
              setRefEntries={setRefEntries}
              selectedDomain={selectedDomain}
              filters={filters}
            />
          )}
        </Box>
      </Fade>
      <DomainContractLookupDialog
        dialogOpen={lookupDomainContractDialogOpen}
        setDialogOpen={setLookupDomainContractDialogOpen}
      />
    </>
  );
};
