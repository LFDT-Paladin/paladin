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
  MenuItem,
  TextField,
  Typography
} from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation } from '@tanstack/react-query';
import { createPrivacyGroupListener } from '../queries/privacyGroups';
import CircleIcon from '@mui/icons-material/Circle';
import { isValidHex, isValidPrivacyGroupListenerName } from '../utils';
import { useNavigate } from 'react-router-dom';

type Props = {
  dialogOpen: boolean
  setDialogOpen: React.Dispatch<React.SetStateAction<boolean>>
}

export const CreatePrivacyGroupListenerDialog: React.FC<Props> = ({
  dialogOpen,
  setDialogOpen,
}) => {

  const { t } = useTranslation();
  const navigate = useNavigate();
  const [listenerName, setListenerName] = useState('');
  const [started, setStarted] = useState(true);
  const [sequenceAbove, setSequenceAbove] = useState(0);
  const [domain, setDomain] = useState('');
  const [group, setGroup] = useState('');
  const [topic, setTopic] = useState('');
  const [excludeLocal, setExcludeLocal] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string>();

  useEffect(() => {
    if (dialogOpen) {
      setErrorMessage(undefined);
      setListenerName('');
      setStarted(true);
      setSequenceAbove(0);
      setDomain('');
      setGroup('');
      setTopic('');
      setExcludeLocal(false);
    }
  }, [dialogOpen]);

  const { mutate: handleSubmit } = useMutation({
    mutationFn: () => createPrivacyGroupListener(
      listenerName,
      started,
      {
        sequenceAbove,
        domain,
        group,
        topic
      },
      {
        excludeLocal
      }
    ),
    onSuccess: () => {
      navigate(`/ui/privacy-groups/listeners/${listenerName}`);
    },
    onError: error => {
      setErrorMessage(error.message);
    }
  });

  const isValidListenerName = isValidPrivacyGroupListenerName(listenerName);
  const isValidGroup = isValidHex(group);
  const canSubmit = isValidListenerName && (group.length === 0 || isValidGroup);

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
          {t('createPrivacyGroupMessageListener')}
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
              fullWidth
              autoComplete="off"
              label={t('listenerName')}
              value={listenerName}
              onChange={event => setListenerName(event.target.value)}
              helperText={listenerName.length > 0 && !isValidListenerName ? t('privacyGroupListenerNameRestrictions') : undefined}
              error={listenerName.length > 0 && !isValidListenerName}
            />
            <TextField
              sx={{ marginBottom: '20px' }}
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
              type="number"
              sx={{ marginBottom: '20px' }}
              fullWidth
              autoComplete="off"
              label={t('sequenceAbove')}
              value={sequenceAbove}
              onChange={event => setSequenceAbove(Number(event.target.value))}
            />
            <TextField
              sx={{ marginBottom: '20px' }}
              fullWidth
              autoComplete="off"
              label={t('domainOptional')}
              value={domain}
              onChange={event => setDomain(event.target.value)}
            />
            <TextField
              sx={{ marginBottom: '20px' }}
              fullWidth
              autoComplete="off"
              label={t('groupOptional')}
              value={group}
              onChange={event => setGroup(event.target.value)}
              error={group.length > 0 && !isValidGroup}
              helperText={group.length > 0 && !isValidGroup ? t('mustBeAValidHex') : undefined}
            />
            <TextField
              sx={{ marginBottom: '20px' }}
              fullWidth
              autoComplete="off"
              label={t('topicOptional')}
              value={topic}
              onChange={event => setTopic(event.target.value)}
            />
            <TextField
              sx={{ marginBottom: '20px' }}
              fullWidth
              label={t('localMessages')}
              value={excludeLocal ? 'exclude' : 'include'}
              onChange={event => setExcludeLocal(event.target.value === 'exclude')}
              select
            >
              <MenuItem value="include">{t('include')}</MenuItem>
              <MenuItem value="exclude">{t('exclude')}</MenuItem>
            </TextField>
          </Box>
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
            onClick={() => setDialogOpen(false)}
          >
            {t('cancel')}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
};
