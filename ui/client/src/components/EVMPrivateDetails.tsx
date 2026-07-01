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

import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Typography
} from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { IPrivateEVMLog, IPrivateEVMReceipt, IPrivateEVMTransaction } from '../interfaces';
import { fetchDecodedCallData, fetchDecodedEvent } from '../queries/abiDecode';
import { JSONBox } from './JSONBox';

type Props = {
  transactionId: string;
  domainReceipt: any;
}

export const EVMPrivateDetails: React.FC<Props> = ({
  transactionId,
  domainReceipt
}) => {

  const { t } = useTranslation();

  const evmTransactionData = domainReceipt?.transaction;
  const evmReceipt = domainReceipt?.receipt;
  const hasTransaction = typeof evmTransactionData === 'object';
  const hasReceipt = typeof evmReceipt === 'object';

  return (
    (hasTransaction || hasReceipt) ? 
      <Accordion elevation={0} disableGutters>
        <AccordionSummary expandIcon={<ExpandMoreIcon />}>
          {t('evmPrivateTransaction')}
        </AccordionSummary>
        <AccordionDetails >
          {hasTransaction ? <EVMPrivateTransaction transactionId={transactionId} evmTransaction={evmTransactionData as IPrivateEVMTransaction}/> : undefined }
          {hasReceipt ? <EVMPrivateReceipt transactionId={transactionId} evmReceipt={evmReceipt as IPrivateEVMReceipt}/> : undefined }
        </AccordionDetails>
      </Accordion>
    : <></>
  );
};

type EVMTxnProps = {
  transactionId: string;
  evmTransaction: IPrivateEVMTransaction;
}

const EVMPrivateTransaction: React.FC<EVMTxnProps> = ({
  transactionId,
  evmTransaction
}) => {

  const { t } = useTranslation();

  const dataString = typeof evmTransaction.data == 'string' ? evmTransaction.data : "";

  const { data: decodedCall } = useQuery({
    enabled: !!dataString,
    queryKey: ["decodeEVMCall", transactionId],
    queryFn: () => fetchDecodedCallData(dataString),
    retry: true
  });

  return (
    <>
      <JSONBox data={evmTransaction} />
      {decodedCall?.data ? <>
        <Typography variant='caption' component='span'>{t('decodedFunction')}</Typography>
        {" "}
        <Typography variant='caption' component='span'><code>{decodedCall.signature}</code></Typography>
        <JSONBox data={decodedCall.data} />
      </> : undefined}
    </>
  );
};

type EVMReceiptProps = {
  transactionId: string;
  evmReceipt: IPrivateEVMReceipt;
}

const EVMPrivateReceipt: React.FC<EVMReceiptProps> = ({
  transactionId,
  evmReceipt
}) => {

  const { t } = useTranslation();

  return (
    <>
      <Typography variant='subtitle2'>{t('evmPrivateReceipt')}</Typography>
      <JSONBox data={{...evmReceipt, logs: undefined}} />
      {
        evmReceipt?.logs?.map((l, i) => 
          <EVMPrivateLog key={`evmlog_${transactionId}_${i}`} transactionId={transactionId} logIndex={i} log={l}/>
        )
      }
    </>
  );
};

type EVMLogProps = {
  transactionId: string;
  logIndex: number;
  log: IPrivateEVMLog;
}

const EVMPrivateLog: React.FC<EVMLogProps> = ({
  transactionId,
  logIndex,
  log
}) => {

  const { t } = useTranslation();

  const { data: decodedEvent } = useQuery({
    enabled: !!log.topics && !!log.data,
    queryKey: ["decodeEVMLog", transactionId, logIndex],
    queryFn: () => fetchDecodedEvent(log.topics!, log.data!),
    retry: true
  });

  return (
    <>
      <Typography variant='caption'>{t('evmPrivateLog', {logIndex})}</Typography>
      <JSONBox data={log} />
      {decodedEvent?.data ? <>
        <Typography variant='caption' component='span'>{t('decodedEvent')}</Typography>
        {" "}
        <Typography variant='caption' component='span'><code>{decodedEvent.signature}</code></Typography>
        <JSONBox data={decodedEvent.data} />
      </> : undefined}
    </>
  );
};
