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
import { customNavigate } from '../utils';
import { useNavigate } from 'react-router-dom';
import { IState } from '../interfaces';
import { pushState } from '../queries/states';

type Props = {
  state: IState
  dialogOpen: boolean
  setDialogOpen: React.Dispatch<React.SetStateAction<boolean>>
}

export const SendStateDialog: React.FC<Props> = ({
  state,
  dialogOpen,
  setDialogOpen,
}) => {

  const { t } = useTranslation();
  const [errorMessage, setErrorMessage] = useState<string>();
  const [recipient, setRecipient] = useState('');
  const [messageId, setMessageId] = useState<string>();
  const [lastRecepient, setLastRecepient] = useState<string>();
  const navigate = useNavigate();

  useEffect(() => {
    if (dialogOpen) {
      setRecipient('');
    }
  }, [dialogOpen]);

  const { refetch: pushMessage } = useQuery({
    queryKey: ['push-state', state, recipient],
    queryFn: () => pushState(state.domain, state.id, recipient),
    retry: false,
    enabled: false
  });

  useEffect(() => {
    if (dialogOpen) {
      setErrorMessage(undefined);
      setMessageId(undefined);
      setLastRecepient(undefined);
    }
  }, [dialogOpen]);

  const handleSubmit = () => {
    setErrorMessage(undefined);
    setMessageId(undefined);
    pushMessage().then(result => {
      if(result.isError) {
        setErrorMessage(t('failedToPushMessageCheckRecipientNode'));
      } else if (result.data !== undefined && result.data.replace(/[0-]/g, '').length === 0) {
        setErrorMessage(t('mustPushStateToAccountInDifferentNode'));
      } else {
        setMessageId(result.data);
        setLastRecepient(recipient);
      }
    });
  };

  const canSubmit = /.+@.+/.test(recipient) && lastRecepient !== recipient;

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
          {t('sendPrivateState')}
          {errorMessage !== undefined &&
            <Alert sx={{ marginTop: '15px' }} variant="filled" severity="warning">{errorMessage}</Alert>}
        </DialogTitle>
        <DialogContent>
          {messageId !== undefined &&
            <Alert variant="filled" severity="success" sx={{ marginBottom: '20px' }}
              action={
                <Button variant="outlined" color="inherit" size="small"
                  onClick={event => customNavigate(`/ui/messages/${messageId}`, event, navigate)}
                >{t('view')}</Button>
              }
            >
              {t('messageValue', { value: messageId })}
            </Alert>}
            <Alert sx={{ marginBottom: '25px'}} variant="filled" severity="warning">
              {t('sendPrivateStateWarning')}
            </Alert>
          <Box sx={{ marginTop: '6px' }}>
            <TextField
              label={t('recipient')}
              autoComplete="OFF"
              sx={{ marginBottom: '20px' }}
              fullWidth
              value={recipient}
              onChange={event => setRecipient(event.target.value)}
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
            {t('send')}
          </Button>
          <Button
            sx={{ minWidth: '100px' }}
            size="large"
            variant="outlined"
            disableElevation
            onClick={() => setDialogOpen(false)}
          >
            {t(lastRecepient !== undefined? 'close' : 'cancel')}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
};
