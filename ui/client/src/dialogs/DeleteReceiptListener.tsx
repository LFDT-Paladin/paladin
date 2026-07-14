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
  Typography
} from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation } from '@tanstack/react-query';
import { deleteReceiptListener } from '../queries/transactions';

type Props = {
  listenerName: string
  refetch: () => any
  dialogOpen: boolean
  setDialogOpen: React.Dispatch<React.SetStateAction<boolean>>
}

export const DeleteReceiptListenerDialog: React.FC<Props> = ({
  listenerName,
  refetch,
  dialogOpen,
  setDialogOpen,
}) => {

  const { t } = useTranslation();
  const [errorMessage, setErrorMessage] = useState<string>();

  useEffect(() => {
    if (dialogOpen) {
      setErrorMessage(undefined);
    }
  }, [dialogOpen]);

  const { mutate: handleSubmit } = useMutation({
    mutationFn: () => deleteReceiptListener(listenerName),
    onSuccess: () => {
      refetch();
      setDialogOpen(false);
    },
    onError: error => {
      setErrorMessage(error.message);
    }
  });

  return (
    <Dialog
      onClose={() => setDialogOpen(false)}
      open={dialogOpen}
      fullWidth
      maxWidth="sm"
    >
      <form onSubmit={(event) => {
        event.preventDefault();
        handleSubmit();
      }}>
        <DialogTitle>
          {t('deleteReceiptListener')}
          {errorMessage && (
            <Alert variant="filled" severity="error">
              {errorMessage}
            </Alert>
          )}
        </DialogTitle>
        <DialogContent>
          <Box sx={{ marginTop: '6px' }}>
            <Typography>{t('deleteReceiptListenerPrompt')}</Typography>
          </Box>
        </DialogContent>
        <DialogActions sx={{ justifyContent: 'center', marginBottom: '15px' }}>
          <Button
            color="error"
            sx={{ minWidth: '100px' }}
            size="large"
            variant="contained"
            disableElevation
            type="submit">
            {t('delete')}
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
