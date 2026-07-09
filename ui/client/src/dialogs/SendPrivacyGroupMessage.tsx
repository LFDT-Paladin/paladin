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
  TextField} from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { isValidHex, isValidUUID } from '../utils';
import { useNavigate } from 'react-router-dom';
import { useMutation } from '@tanstack/react-query';
import { sendPrivacyGroupMessage } from '../queries/privacyGroups';

type Props = {
  preSelectedPrivacyGroupId?: string
  dialogOpen: boolean
  setDialogOpen: React.Dispatch<React.SetStateAction<boolean>>
}

export const SendPrivacyGroupMessageDialog: React.FC<Props> = ({
  preSelectedPrivacyGroupId,
  dialogOpen,
  setDialogOpen,
}) => {

  const { t } = useTranslation();
  const [privacyGroupId, setPrivacyGroupId] = useState(preSelectedPrivacyGroupId ?? '');
  const [topic, setTopic] = useState('');
  const [correlationId, setCorrelationId] = useState('');
  const [data, setData] = useState('');
  const [errorMessage, setErrorMessage] = useState<string>();
  const navigate = useNavigate();

  useEffect(() => {
    if (dialogOpen) {
      setTopic('');
      setCorrelationId('');
      setData('');
    }
  }, [dialogOpen]);

  const getData = () => {
    try {
      return JSON.parse(data);
    } catch(_err) {
      return data;
    }
  }

  const { mutate: handleSubmit } = useMutation({
    mutationFn: () => sendPrivacyGroupMessage(privacyGroupId, topic, getData(), correlationId.length > 0? correlationId : undefined),
    onSuccess: messageId => {
      navigate(`/ui/privacy-groups/messages/${messageId}`);
    },
    onError: error => {
      setErrorMessage(error.message);
    }
  });

  const isValidPrivacyGroupId = isValidHex(privacyGroupId);

  const canSubmit = privacyGroupId.length > 0 && isValidPrivacyGroupId && topic.length > 0
    && (correlationId.length === 0 || isValidUUID(correlationId))
    && data.length > 0;

  return (
    <Dialog
      onClose={() => setDialogOpen(false)}
      open={dialogOpen}
      fullWidth
      maxWidth="xs"
    >
      <form onSubmit={(event) => {
        event.preventDefault();
        handleSubmit();
      }}>
        <DialogTitle>
          {t('sendPrivacyGroupMessage')}
          {errorMessage && (
            <Alert variant="filled" severity="error">
              {errorMessage}
            </Alert>
          )}
        </DialogTitle>
        <DialogContent>
          <Box sx={{ marginTop: '6px' }}>
            <TextField
              sx={{ marginBottom: '20px' }}
              label={t('privacyGroupId')}
              autoComplete="off"
              fullWidth
              value={privacyGroupId}
              onChange={event => setPrivacyGroupId(event.target.value)}
              helperText={privacyGroupId.length > 0 && !isValidPrivacyGroupId? t('mustBeAValidHex') : undefined}
              error={privacyGroupId.length > 0 && !isValidPrivacyGroupId}
            />
            <TextField
              sx={{ marginBottom: '20px' }}
              label={t('topic')}
              autoComplete="off"
              fullWidth
              value={topic}
              onChange={event => setTopic(event.target.value)}
            />
            <TextField
              sx={{ marginBottom: '20px' }}
              label={t('correlationIdOptional')}
              autoComplete="off"
              fullWidth
              value={correlationId}
              onChange={event => setCorrelationId(event.target.value)}
              error={correlationId.length > 0 && !isValidUUID(correlationId)}
              helperText={correlationId.length > 0 && !isValidUUID(correlationId) ? t('mustBeAValidUUID') : undefined}
            />
            <TextField
              fullWidth
              label={t('data')}
              multiline
              rows={5}
              value={data}
              onChange={event => setData(event.target.value)}
              helperText={t('formatValue', { value: typeof getData()})}
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
            {t('cancel')}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
};
