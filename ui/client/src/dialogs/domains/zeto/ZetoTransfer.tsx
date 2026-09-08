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
  Stack,
  TextField
} from '@mui/material';
import { useMutation } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { TransactionType } from '../../../interfaces';
import { sendTransaction } from '../../../queries/transactions';
import { customNavigate, encodeHex } from '../../../utils';
import { useNavigate } from 'react-router-dom';
import { AppRouteFactory } from '../../../routes';

type Props = {
  contractAddress: string;
  onClose: () => void
};

const transferAbi = {
  inputs: [
    {
      components: [
        {
          internalType: 'string',
          name: 'to',
          type: 'string',
        },
        {
          internalType: 'uint256',
          name: 'amount',
          type: 'uint256',
        },
        {
          internalType: 'bytes',
          name: 'data',
          type: 'bytes',
        },
      ],
      internalType: 'struct IZetoFungible.TransferParam[]',
      name: 'transfers',
      type: 'tuple[]',
    },
  ],
  name: 'transfer',
  outputs: [],
  stateMutability: 'nonpayable',
  type: 'function',
};

export const ZetoTransferDialog: React.FC<Props> = ({
  contractAddress,
  onClose,
}) => {
  const { t } = useTranslation();
  const [sender, setSender] = useState('');
  const [recipient, setRecipient] = useState('');
  const [amount, setAmount] = useState('');
  const [data, setData] = useState('');
  const [errorMessage, setErrorMessage] = useState<string>();
  const navigate = useNavigate();

  const { mutate, error, data: transactionId } = useMutation({
    mutationFn: () =>
      sendTransaction({
        type: TransactionType.PRIVATE,
        from: sender,
        to: contractAddress,
        abi: [transferAbi],
        function: 'transfer',
        data: {
          transfers: [
            {
              to: recipient,
              amount,
              data: encodeHex(data),
            }
          ]
        }
      })
  });

  useEffect(() => {
    if (error !== null) {
      setErrorMessage(t('transferFailed'));
    }
  }, [error]);

  const canSubmit =
    sender.length > 0 &&
    recipient.length > 0 &&
    amount.length > 0 &&
    !isNaN(parseInt(amount));

  return (
    <Dialog
      open
      onClose={onClose}
      fullWidth
      maxWidth="sm"
    >
      <form
        onSubmit={(event) => {
          event.preventDefault();
          mutate();
        }}
      >
        <DialogTitle sx={{ textAlign: 'center' }}>
          {t('transfer')}
          <Box sx={{ marginTop: '10px' }}>
            {errorMessage !== undefined && (
              <Alert variant="filled" severity="error">
                {errorMessage}
              </Alert>
            )}
          </Box>
        </DialogTitle>
        <DialogContent>
          <Stack spacing={3} sx={{ marginTop: '5px' }}>
            {transactionId !== undefined &&
            <Alert variant="filled" severity="success"
              action={
                <Button variant="outlined" color="inherit" size="small"
                onClick={event => customNavigate(AppRouteFactory.getPath('Transaction', { hashOrId: transactionId }, { back: 'domains' }), event, navigate)}
                >{t('view')}</Button>
              }
            >
              {t('transactionValue', { value: transactionId })}
            </Alert>}
            <TextField
              fullWidth
              disabled
              label={t('contractAddress')}
              autoComplete="off"
              value={contractAddress}
            />
            <TextField
              fullWidth
              label={t('from')}
              autoComplete="off"
              value={sender}
              onChange={(event) => setSender(event.target.value)}
            />
            <TextField
              fullWidth
              label={t('to')}
              autoComplete="off"
              value={recipient}
              onChange={(event) => setRecipient(event.target.value)}
            />
            <TextField
              fullWidth
              label={t('amount')}
              autoComplete="off"
              value={amount}
              onChange={(event) => setAmount(event.target.value)}
            />
            <TextField
              fullWidth
              label={t('dataOptional')}
              autoComplete="off"
              value={data}
              onChange={(event) => setData(event.target.value)}
            />
          </Stack>
        </DialogContent>
        <DialogActions sx={{ justifyContent: 'center', paddingBottom: '20px' }}>
          <Button
            sx={{ minWidth: '100px' }}
            size="large"
            variant="contained"
            disableElevation
            disabled={!canSubmit}
            type="submit"
          >
            {t('transfer')}
          </Button>
          <Button
            sx={{ minWidth: '100px' }}
            size="large"
            variant="outlined"
            disableElevation
            onClick={() => onClose()}
          >
            {t('close')}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
};
