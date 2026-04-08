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
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  Radio,
  RadioGroup,
  TextField
} from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { fetchTransaction, fetchTransactionReceipt } from '../queries/transactions';
import { isValidTransactionHash, isValidUUID } from '../utils';
import { useNavigate } from 'react-router-dom';

type Props = {
  dialogOpen: boolean
  setDialogOpen: React.Dispatch<React.SetStateAction<boolean>>
}

export const TransactionLookupDialog: React.FC<Props> = ({
  dialogOpen,
  setDialogOpen
}) => {

  const { t } = useTranslation();
  const [selectedType, setSelectedType] = useState<'bth' | 'pti'>('bth');
  const [notFound, setNotFound] = useState(false);
  const [blockchainTransactionHash, setBlockchainTransactionHash] = useState('');
  const [paladinTransactionId, setPaladinTransactionId] = useState('');
  const navigate = useNavigate();

  useEffect(() => {
    if (dialogOpen) {
      setSelectedType('bth');
      setBlockchainTransactionHash('');
      setPaladinTransactionId('');
    }
  }, [dialogOpen]);

  const { refetch: blockchainTransactionByHash } = useQuery({
    queryKey: ["blockchainTransactionByHash", blockchainTransactionHash],
    queryFn: () => fetchTransaction(blockchainTransactionHash),
    enabled: false,
    refetchOnMount: false,
    retry: false
  });

  const { refetch: paladinTransactionById } = useQuery({
    queryKey: ["paladinTransactionById", paladinTransactionId],
    queryFn: () => fetchTransactionReceipt(paladinTransactionId),
    enabled: false,
    refetchOnMount: false,
    retry: false
  });

  const handleSubmit = () => {
    setNotFound(false);
    if (selectedType === 'bth') {
      blockchainTransactionByHash().then(result => {
        if (result.isSuccess) {
          navigate(`/ui/transactions/${blockchainTransactionHash}`);
        } else {
          setNotFound(true);
        }
      });
    } else {
      paladinTransactionById().then(result => {
        if (result.isSuccess && result.data !== null) {
          navigate(`/ui/transactions/${result.data.transactionHash}/${paladinTransactionId}`);
        } else {
          setNotFound(true);
        }
      });
    }
  };

  const canSubmit = (selectedType === 'bth' && isValidTransactionHash(blockchainTransactionHash))
    || (selectedType === 'pti' && isValidUUID(paladinTransactionId));

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
          {t('lookupTransaction')}
          {notFound &&
            <Alert sx={{ marginTop: '15px' }} variant="filled" severity="warning">{t('transactionNotFound')}</Alert>}

        </DialogTitle>
        <DialogContent>

          <RadioGroup
            value={selectedType}
            onChange={event => setSelectedType((event.target as HTMLInputElement).value as any)}
          >
            <FormControlLabel value="bth" control={<Radio />} label={t('blockchainTransactionHash')} />
            <TextField
              autoComplete="OFF"
              sx={{ marginBottom: '20px' }}
              fullWidth
              value={blockchainTransactionHash}
              onChange={event => {
                if (selectedType !== 'bth') {
                  setSelectedType('bth');
                }
                setBlockchainTransactionHash(event.target.value);
              }}
            />
            <FormControlLabel value="pti" control={<Radio />} label={t('paladinTransactionId')} />
            <TextField
              autoComplete="OFF"
              fullWidth
              value={paladinTransactionId}
              onChange={event => {
                if (selectedType !== 'pti') {
                  setSelectedType('pti');
                }
                setPaladinTransactionId(event.target.value);
              }}
            />
          </RadioGroup>


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
