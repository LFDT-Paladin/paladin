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
  Box,
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
import { Dispatch, SetStateAction, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ITransactionPagingReference } from '../interfaces';

type Props = {
  dialogOpen: boolean
  setDialogOpen: React.Dispatch<React.SetStateAction<boolean>>
  fromBlock: number | undefined
  setFromBlock: React.Dispatch<React.SetStateAction<number | undefined>>
  setPage: React.Dispatch<React.SetStateAction<number>>
  setRefEntries: Dispatch<SetStateAction<ITransactionPagingReference[]>>
}

export const FromBlockDialog: React.FC<Props> = ({
  dialogOpen,
  setDialogOpen,
  fromBlock,
  setFromBlock,
  setPage,
  setRefEntries
}) => {

  const { t } = useTranslation();
  const [selectedType, setSelectedType] = useState<'latest' | 'blockNumber'>(fromBlock === undefined? 'latest' : 'blockNumber');
  const [selectedBlockNumber, setSelectedBlockNumber] = useState('');

  useEffect(() => {
    if(dialogOpen) {
      setSelectedType(fromBlock === undefined? 'latest' : 'blockNumber');
      setSelectedBlockNumber(fromBlock?.toString() ?? '');
    }
  }, [dialogOpen]);

  const handleSubmit = () => {
    if(selectedType === 'latest') {
      if(fromBlock !== undefined) {
        setPage(0);
        setRefEntries([]);
        setFromBlock(undefined);
      }
    } else if(selectedType === 'blockNumber') {
      if(fromBlock === undefined) {
        setPage(0);
        setRefEntries([]);
        setFromBlock(Number(selectedBlockNumber));
      }
    }
    setDialogOpen(false);
  };

  const selectedTypeChanged = (selectedType === 'latest' && fromBlock !== undefined)
  || (selectedType === 'blockNumber' && fromBlock === undefined);

  const blockNumberChanged = fromBlock?.toString() !== selectedBlockNumber;

  const canSubmit = (selectedType === 'latest'
  || (selectedType === 'blockNumber' && /^\d+$/.test(selectedBlockNumber)))
  && (selectedTypeChanged || (selectedType === 'blockNumber' && blockNumberChanged));

  return (
    <Dialog
      onClose={() => setDialogOpen(false)}
      open={dialogOpen}
    >
      <form onSubmit={(event) => {
        event.preventDefault();
        handleSubmit();
      }}>
        <DialogTitle>
          {t('startFrom')}
        </DialogTitle>
        <DialogContent>
          <RadioGroup
            value={selectedType}
            onChange={event => setSelectedType((event.target as HTMLInputElement).value as any)}
          >
            <FormControlLabel value="latest" control={<Radio />} label={t('latestBlock')} />
            <Box sx={{
              marginTop: '10px',
              disaply: 'flex',
              
            }}>
            <FormControlLabel value="blockNumber" control={<Radio />} label={t('blockNumber')} />
            <TextField
              autoComplete="OFF"
              sx={{ width: '120px', marginTop: '2px'}}
              size="small"
              value={selectedBlockNumber}
              onChange={event => {
                if(selectedType !== 'blockNumber') {
                  setSelectedType('blockNumber');
                }
                setSelectedBlockNumber(event.target.value)
              }}
            />
            </Box>
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
            {t('set')}
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
