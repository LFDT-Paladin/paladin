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
  TextField
} from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { isValidHex } from '../utils';
import { useNavigate } from 'react-router-dom';
import { getState } from '../queries/states';
import { AppRouteFactory } from '../routes';

type Props = {
  domain: string
  schemaId: string
  onClose: () => void
}

export const StateLookupDialog: React.FC<Props> = ({
  domain,
  schemaId,
  onClose,
}) => {

  const { t } = useTranslation();
  const [notFound, setNotFound] = useState(false);
  const [stateId, setStateId] = useState('');
  const navigate = useNavigate();

  const { refetch: state } = useQuery({
    queryKey: ['state', domain, schemaId, stateId],
    queryFn: () => getState(domain, schemaId, stateId!),
    retry: false,
    enabled: false
  });


  const handleSubmit = () => {
    setNotFound(false);
    state().then(result => {
      if (result.isSuccess && result.data !== null) {
        navigate(AppRouteFactory.getPath('State', { domain, schema: schemaId, id: stateId }));
      } else {
        setNotFound(true);
      }
    });
  };

  const canSubmit = isValidHex(stateId);

  return (
    <Dialog
      onClose={onClose}
      open
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
            <Alert sx={{ marginTop: '15px' }} variant="filled" severity="warning">{t('stateNotFound')}</Alert>}
        </DialogTitle>
        <DialogContent>
          <Box sx={{ marginTop: '6px' }}>
            <TextField
              label={t('stateId')}
              autoComplete="off"
              fullWidth
              value={stateId}
              onChange={event => setStateId(event.target.value)}
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
            onClick={() => onClose()}
          >
            {t('cancel')}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
};
