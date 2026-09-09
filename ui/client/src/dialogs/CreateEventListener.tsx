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
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  MenuItem,
  Stack,
  TextField,
  Typography
} from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation } from '@tanstack/react-query';
import { createEventListener } from '../queries/transactions';
import CircleIcon from '@mui/icons-material/Circle';
import { isValidHex, isValidPrivacyGroupListenerName } from '../utils';
import { IABIEntry, IEventListenerOptions } from '../interfaces';
import { useNavigate } from 'react-router-dom';
import { AppRouteFactory } from '../routes';

type Props = {
  onClose: () => void
}

export const CreateEventListenerDialog: React.FC<Props> = ({
  onClose,
}) => {

  const { t } = useTranslation();
  const navigate = useNavigate();
  const [listenerName, setListenerName] = useState('');
  const [started, setStarted] = useState(true);
  const [abiText, setAbiText] = useState('');
  const [address, setAddress] = useState('');
  const [fromBlock, setFromBlock] = useState('');
  const [batchSize, setBatchSize] = useState('');
  const [batchTimeout, setBatchTimeout] = useState('');
  const [errorMessage, setErrorMessage] = useState<string>();

  const parseAbi = (): IABIEntry[] | undefined => {
    if (abiText.trim().length === 0) {
      return undefined;
    }
    try {
      const parsed = JSON.parse(abiText);
      if (!Array.isArray(parsed)) {
        return undefined;
      }
      return parsed;
    } catch {
      return undefined;
    }
  };

  const parsedAbi = parseAbi();
  const isValidAbi = abiText.trim().length === 0 || parsedAbi !== undefined;
  const isValidListenerName = isValidPrivacyGroupListenerName(listenerName);
  const isValidAddress = address.length === 0 || isValidHex(address);
  const isValidFromBlock =
    fromBlock.length === 0 ||
    fromBlock === 'latest' ||
    /^\d+$/.test(fromBlock);
  const isValidBatchTimeout = batchTimeout.length === 0 || /^(\d+h)?(\d+m)?(\d+s)?(\d+ms)?(\d+ns)?$/.test(batchTimeout);
  const canSubmit = isValidListenerName && parsedAbi !== undefined && isValidAddress && isValidFromBlock && isValidBatchTimeout;

  const { mutate: handleSubmit } = useMutation({
    mutationFn: () => {
      const options: IEventListenerOptions = {};
      if (fromBlock.length > 0) {
        options.fromBlock = fromBlock === 'latest' ? 'latest' : Number(fromBlock);
      }
      if (batchSize.trim().length > 0) {
        options.batchSize = Number(batchSize);
      }
      if (batchTimeout.trim().length > 0) {
        options.batchTimeout = batchTimeout.trim();
      }
      return createEventListener({
        name: listenerName,
        started,
        sources: [{
          abi: parsedAbi!,
          ...(address.length > 0 ? { address } : {})
        }],
        options
      });
    },
    onSuccess: () => {
      navigate(AppRouteFactory.getPath('EventListenerEntry', { id: listenerName }));
    },
    onError: error => {
      setErrorMessage(
        error.message.includes('already exists')
          ? t('listenerNameAlreadyExists')
          : error.message
      );
    }
  });

  return (
    <Dialog
      onClose={onClose}
      open
      fullWidth
      maxWidth="xs"
    >
      <form onSubmit={(event) => {
        event.preventDefault();
        handleSubmit();
      }}>
        <DialogTitle>
          {t('createEventListener')}
          {errorMessage && (
            <Alert variant="filled" severity="error">
              {errorMessage}
            </Alert>
          )}
        </DialogTitle>
        <DialogContent>
          <Stack spacing={3} sx={{ marginTop: '6px' }}>
            <TextField
              fullWidth
              autoComplete="off"
              label={t('listenerName')}
              value={listenerName}
              onChange={event => setListenerName(event.target.value)}
              helperText={listenerName.length > 0 && !isValidListenerName ? t('eventListenerNameRestrictions') : undefined}
              error={listenerName.length > 0 && !isValidListenerName}
            />
            <TextField
              fullWidth
              label={t('initialStatus')}
              value={started ? 'started' : 'stopped'}
              onChange={event => setStarted(event.target.value === 'started')}
              select
            >
              <MenuItem value="started">
                <Box sx={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: '8px'
                }}>
                  <CircleIcon sx={{ fontSize: '16px' }} color="success" />
                  <Typography>{t('started')}</Typography>
                </Box>
              </MenuItem>
              <MenuItem value="stopped">
                <Box sx={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: '8px'
                }}>
                  <CircleIcon sx={{ fontSize: '16px' }} color="warning" />
                  <Typography>{t('stopped')}</Typography>
                </Box>
              </MenuItem>
            </TextField>
            <TextField
              fullWidth
              multiline
              minRows={4}
              maxRows={4}
              autoComplete="off"
              label={t('abi')}
              value={abiText}
              onChange={event => setAbiText(event.target.value)}
              error={abiText.trim().length > 0 && !isValidAbi}
              helperText={abiText.trim().length > 0 && !isValidAbi ? t('invalidABI') : undefined}
            />
            <TextField
              fullWidth
              autoComplete="off"
              label={t('addressOptional')}
              value={address}
              onChange={event => setAddress(event.target.value)}
              error={address.length > 0 && !isValidAddress}
              helperText={address.length > 0 && !isValidAddress ? t('mustBeAValidHex') : undefined}
            />
            <TextField
              fullWidth
              autoComplete="off"
              label={t('fromBlockOptional')}
              value={fromBlock}
              onChange={event => setFromBlock(event.target.value)}
              error={fromBlock.length > 0 && !isValidFromBlock}
              helperText={fromBlock.length > 0 && !isValidFromBlock ? t('invalidFromBlock') : undefined}
            />
            <TextField
              type="number"
              sx={{
                '& input::-webkit-outer-spin-button, & input::-webkit-inner-spin-button': {
                  display: 'none',
                  WebkitAppearance: 'none',
                  margin: 0,
                },
                '& input[type=number]': {
                  MozAppearance: 'textfield',
                }
              }}
              fullWidth
              autoComplete="off"
              label={t('batchSizeOptional')}
              value={batchSize}
              onChange={event => setBatchSize(event.target.value)}
            />
            <TextField
              fullWidth
              autoComplete="off"
              label={t('batchTimeoutOptional')}
              value={batchTimeout}
              onChange={event => setBatchTimeout(event.target.value)}
              error={batchTimeout.length > 0 && !isValidBatchTimeout}
              helperText={batchTimeout.length > 0 && !isValidBatchTimeout ? t('invalidDuration') : undefined}
            />
          </Stack>
        </DialogContent>
        <DialogActions sx={{ justifyContent: 'center', marginBottom: '15px' }}>
          <Button
            sx={{ minWidth: '100px' }}
            size="large"
            variant="contained"
            disabled={!canSubmit}
            type="submit">
            {t('create')}
          </Button>
          <Button
            sx={{ minWidth: '100px' }}
            size="large"
            variant="outlined"
            onClick={() => onClose()}
          >
            {t('cancel')}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
};
