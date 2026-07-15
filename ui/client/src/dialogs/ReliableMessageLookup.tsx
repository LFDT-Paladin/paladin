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
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { isValidUUID } from '../utils';
import { useNavigate } from 'react-router-dom';
import { getMessage } from '../queries/transport';

type Props = {
  dialogOpen: boolean
  setDialogOpen: React.Dispatch<React.SetStateAction<boolean>>
}

export const ReliableMessageLookupDialog: React.FC<Props> = ({
  dialogOpen,
  setDialogOpen,
}) => {

  const { t } = useTranslation();
  const [notFound, setNotFound] = useState(false);
  const [id, setId] = useState('');
  const navigate = useNavigate();

  useEffect(() => {
    if (dialogOpen) {
      setId('');
    }
  }, [dialogOpen]);

  const { refetch: messageById } = useQuery({
    queryKey: [`message-by-id-${id}`],
    queryFn: () => getMessage(id!),
    retry: false,
    enabled: false
  });

  const handleSubmit = () => {
    setNotFound(false);
    messageById().then(result => {
      if (result.isSuccess && result.data !== null) {
        navigate(`/ui/transports/messages/${id}`);
      } else {
        setNotFound(true);
      }
    });
  };

  const canSubmit = isValidUUID(id);

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
            <Alert sx={{ marginTop: '15px' }} variant="filled" severity="warning">{t('messageNotFound')}</Alert>}
        </DialogTitle>
        <DialogContent>
          <Box sx={{ marginTop: '6px' }}>
            <TextField
              label={t('messageId')}
              autoComplete="off"
              sx={{ marginBottom: '20px' }}
              fullWidth
              value={id}
              onChange={event => setId(event.target.value)}
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
