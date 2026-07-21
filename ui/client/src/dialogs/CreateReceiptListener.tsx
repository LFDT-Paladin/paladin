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
import { createReceiptListener } from '../queries/transactions';
import CircleIcon from '@mui/icons-material/Circle';
import { isValidPrivacyGroupListenerName } from '../utils';
import { IReceiptListenerFilters, IReceiptListenerOptions, TransactionType } from '../interfaces';
import { useNavigate } from 'react-router-dom';

type Props = {
  onClose: () => void
}

export const CreateReceiptListenerDialog: React.FC<Props> = ({
  onClose,
}) => {

  const { t } = useTranslation();
  const navigate = useNavigate();
  const [listenerName, setListenerName] = useState('');
  const [started, setStarted] = useState(true);
  const [sequenceAbove, setSequenceAbove] = useState('');
  const [type, setType] = useState('all');
  const [domain, setDomain] = useState('');
  const [domainReceipts, setDomainReceipts] = useState(false);
  const [incompleteStateReceiptBehavior, setIncompleteStateReceiptBehavior] = useState('default');
  const [errorMessage, setErrorMessage] = useState<string>();

  const isValidListenerName = isValidPrivacyGroupListenerName(listenerName);
  const isValidSequenceAbove = sequenceAbove.length === 0 || /^\d+$/.test(sequenceAbove);
  const canSubmit = isValidListenerName && isValidSequenceAbove;

  const { mutate: handleSubmit } = useMutation({
    mutationFn: () => {
      const filters: IReceiptListenerFilters = {};
      if (sequenceAbove.length > 0) {
        filters.sequenceAbove = Number(sequenceAbove);
      }
      if (type.length > 0 && type !== 'all') {
        filters.type = type as TransactionType;
      }
      if (domain.length > 0) {
        filters.domain = domain;
      }
      const options: IReceiptListenerOptions = {
        domainReceipts,
      };
      if (incompleteStateReceiptBehavior.length > 0 && incompleteStateReceiptBehavior !== 'default') {
        options.incompleteStateReceiptBehavior = incompleteStateReceiptBehavior as IReceiptListenerOptions['incompleteStateReceiptBehavior'];
      }
      return createReceiptListener({
        name: listenerName,
        started,
        filters,
        options
      });
    },
    onSuccess: () => {
      navigate(`/ui/listeners/receipts/${listenerName}`);
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
          {t('createReceiptListener')}
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
              helperText={listenerName.length > 0 && !isValidListenerName ? t('receiptListenerNameRestrictions') : undefined}
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
              autoComplete="off"
              label={t('sequenceAboveOptional')}
              value={sequenceAbove}
              onChange={event => setSequenceAbove(event.target.value)}
              error={sequenceAbove.length > 0 && !isValidSequenceAbove}
              helperText={sequenceAbove.length > 0 && !isValidSequenceAbove ? t('mustBeAValidNumber') : undefined}
            />
            <TextField
              fullWidth
              label={t('typeOptional')}
              value={type}
              onChange={event => setType(event.target.value)}
              select
            >
              <MenuItem value="all">{t('all')}</MenuItem>
              <MenuItem value={TransactionType.PUBLIC}>{t('public')}</MenuItem>
              <MenuItem value={TransactionType.PRIVATE}>{t('private')}</MenuItem>
            </TextField>
            <TextField
              fullWidth
              autoComplete="off"
              label={t('domainOptional')}
              value={domain}
              onChange={event => setDomain(event.target.value)}
            />
            <TextField
              fullWidth
              label={t('domainReceipts')}
              value={domainReceipts ? 'true' : 'false'}
              onChange={event => setDomainReceipts(event.target.value === 'true')}
              select
            >
              <MenuItem value="false">{t('false')}</MenuItem>
              <MenuItem value="true">{t('true')}</MenuItem>
            </TextField>
            <TextField
              fullWidth
              label={t('incompleteStateReceiptBehaviorOptional')}
              value={incompleteStateReceiptBehavior}
              onChange={event => setIncompleteStateReceiptBehavior(event.target.value)}
              select
            >
              <MenuItem value="default">{t('default')}</MenuItem>
              <MenuItem value="block_contract">{t('blockContract')}</MenuItem>
              <MenuItem value="process">{t('process')}</MenuItem>
              <MenuItem value="complete_only">{t('completeOnly')}</MenuItem>
            </TextField>
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
