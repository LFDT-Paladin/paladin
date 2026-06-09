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
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  TextField
} from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { isValidAddress } from '../utils';
import { useNavigate } from 'react-router-dom';
import { getDomainContractByAddress } from '../queries/domains';

type Props = {
  dialogOpen: boolean
  setDialogOpen: React.Dispatch<React.SetStateAction<boolean>>
}

export const DomainContractLookupDialog: React.FC<Props> = ({
  dialogOpen,
  setDialogOpen,
}) => {

  const { t } = useTranslation();
  const [notFound, setNotFound] = useState(false);
  const [address, setAddress] = useState('');
  const navigate = useNavigate();

  useEffect(() => {
    if (dialogOpen) {
      setAddress('');
    }
  }, [dialogOpen]);

  const { refetch: domainContractByAddress } = useQuery({
    queryKey: [`domain-contract-${address}`],
    queryFn: () => getDomainContractByAddress(address),
    enabled: isValidAddress(address),
    refetchOnMount: false,
    retry: false
  });

  const handleSubmit = () => {
    setNotFound(false);
    domainContractByAddress().then(result => {
      if (result.isSuccess) {
        navigate(`/ui/domains/${address}`);
      } else {
        setNotFound(true);
      }
    });
  };

  const canSubmit = isValidAddress(address);

  return (
    <Dialog
      onClose={() => setDialogOpen(false)}
      open={dialogOpen}
      PaperProps={{ sx: { width: '680px' } }}
      fullWidth
      maxWidth="md"
    >
      <form onSubmit={(event) => {
        event.preventDefault();
        handleSubmit();
      }}>
        <DialogTitle>
          {t('lookup')}
          {notFound &&
            <Alert sx={{ marginTop: '15px' }} variant="filled" severity="warning">{t('domainSmartContractNotFound')}</Alert>}
        </DialogTitle>
        <DialogContent>
          <Box sx={{ marginTop: '6px' }}>
            <TextField
              label={t('contractAddress')}
              autoComplete="OFF"
              sx={{ marginBottom: '20px' }}
              fullWidth
              value={address}
              onChange={event => setAddress(event.target.value)}
            />
          </Box>
        </DialogContent>
        <DialogActions sx={{ justifyContent: 'center', marginBottom: '15px' }}>
          <Button
            sx={{ minWidth: '100px' }}
            size="large"
            variant="contained"
            disableElevation
            disabled={!canSubmit}
            type="submit">
            {t('lookup')}
          </Button>
          <Button
            sx={{ minWidth: '100px' }}
            size="large"
            variant="outlined"
            disableElevation
            onClick={() => setDialogOpen(false)}
          >
            {t('cancel')}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
};
