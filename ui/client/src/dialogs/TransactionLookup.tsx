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
import { fetchPaladinTransaction, fetchTransaction, fetchTransactionReceipt } from '../queries/transactions';
import { isValidTransactionHash, isValidUUID } from '../utils';
import { useNavigate } from 'react-router-dom';

type Props = {
  dialogOpen: boolean
  setDialogOpen: React.Dispatch<React.SetStateAction<boolean>>
  label: string
}

export const TransactionLookupDialog: React.FC<Props> = ({
  dialogOpen,
  setDialogOpen,
  label
}) => {

  const { t } = useTranslation();
  const [notFound, setNotFound] = useState(false);
  const [hashOrId, setHashOrId] = useState('');
  const navigate = useNavigate();

  useEffect(() => {
    if (dialogOpen) {
      setHashOrId('');
    }
  }, [dialogOpen]);

  const { refetch: blockchainTransactionByHash } = useQuery({
    queryKey: ["blockchainTransactionByHash", hashOrId],
    queryFn: () => fetchTransaction(hashOrId),
    enabled: isValidTransactionHash(hashOrId),
    refetchOnMount: false,
    retry: false
  });

  const { refetch: paladinTransactionById } = useQuery({
    queryKey: ["paladinTransactionById", hashOrId],
    queryFn: () => fetchPaladinTransaction(hashOrId),
    enabled: isValidUUID(hashOrId),
    refetchOnMount: false,
    retry: false
  });

  const handleSubmit = () => {
    setNotFound(false);
    if (isValidTransactionHash(hashOrId)) {
      blockchainTransactionByHash().then(result => {
        if (result.isSuccess) {
          navigate(`/ui/transactions/${hashOrId}`);
        } else {
          setNotFound(true);
        }
      });
    } else if (isValidUUID(hashOrId)) {
      paladinTransactionById().then(result => {
        if (result.isSuccess && result.data !== null) {
          navigate(`/ui/transactions/${hashOrId}`);
        } else {
          setNotFound(true);
        }
      });
    }
  };

  const canSubmit = isValidTransactionHash(hashOrId) || isValidUUID(hashOrId);

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
            <Alert sx={{ marginTop: '15px' }} variant="filled" severity="warning">{t('transactionNotFound')}</Alert>}
        </DialogTitle>
        <DialogContent>
          <Box sx={{ marginTop: '6px' }}>

            <TextField
              label={label}
              autoComplete="OFF"
              sx={{ marginBottom: '20px' }}
              fullWidth
              value={hashOrId}
              onChange={event => setHashOrId(event.target.value)}
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
