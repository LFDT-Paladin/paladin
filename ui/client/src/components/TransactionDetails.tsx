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
  AccordionSummary
} from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { ITransactionReceipt } from '../interfaces';
import { fetchDomainReceipt } from '../queries/domains';
import { fetchStateReceipt } from '../queries/states';
import { EVMPrivateDetails } from './EVMPrivateDetails';
import { JSONBox } from './JSONBox';
import { fetchPaladinTransactionFull } from '../queries/transactions';
import { useEffect, useState } from 'react';

type Props = {
  receipt: ITransactionReceipt
}

export const PaladinTransactionsDetails: React.FC<Props> = ({
  receipt
}) => {

  const [receiptExpanded, setReceiptExpanded] = useState(false);
  const { t } = useTranslation();

  const { data: paladinTransaction, isFetched: paladinTransactionFetched } = useQuery({
    queryKey: ['paladin-transaction-full', receipt.id],
    queryFn: () => fetchPaladinTransactionFull(receipt.id),
    retry: false
  });

  const { data: domainReceipt } = useQuery({
    enabled: receipt.domain !== undefined,
    queryKey: ['domain-receipt', receipt.domain, receipt.id],
    queryFn: () => fetchDomainReceipt(receipt.domain!, receipt.id),
    retry: false
  });

  const { data: stateReceipt } = useQuery({
    queryKey: ['state-receipt', receipt.id],
    queryFn: () => fetchStateReceipt(receipt.id),
    retry: false
  });

  useEffect(() => {
    if(paladinTransaction !== null ){
      setReceiptExpanded(true);
    }
  }, [paladinTransaction]);

  if (!paladinTransactionFetched) {
    return <></>;
  }

  return (
    <>
      {paladinTransaction &&
        <Accordion elevation={0} disableGutters defaultExpanded>
          <AccordionSummary expandIcon={<ExpandMoreIcon />}>
            {t('details')}
          </AccordionSummary>
          <AccordionDetails >
            <JSONBox data={paladinTransaction} />
          </AccordionDetails>
        </Accordion>}

      <Accordion elevation={0} disableGutters defaultExpanded={receiptExpanded}>
        <AccordionSummary expandIcon={<ExpandMoreIcon />}>
          {t('receipt')}
        </AccordionSummary>
        <AccordionDetails >
          <JSONBox data={receipt} />
        </AccordionDetails>
      </Accordion>

      {domainReceipt !== undefined && <>
        <EVMPrivateDetails transactionId={receipt.id} domainReceipt={domainReceipt} />
        <Accordion elevation={0} disableGutters>
          <AccordionSummary expandIcon={<ExpandMoreIcon />}>
            {t('domainReceipt')}
          </AccordionSummary>
          <AccordionDetails >
            <JSONBox data={domainReceipt} />
          </AccordionDetails>
        </Accordion>
      </>}

      {!(stateReceipt?.none === true) &&
        <Accordion elevation={0} disableGutters>
          <AccordionSummary expandIcon={<ExpandMoreIcon />}>
            {t('stateReceipt')}
          </AccordionSummary>
          <AccordionDetails >
            <JSONBox data={stateReceipt} />
          </AccordionDetails>
        </Accordion>}
    </>
  );
};
