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

import { Alert, Box, Button, Fade, Grid2, Tab, Tabs, Typography } from "@mui/material";
import { useQuery } from "@tanstack/react-query";
import { fetchPaladinTransaction, fetchEnrichedTransaction, fetchTransactionReceipt } from "../queries/transactions";
import { useEffect, useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import { capitalize, getShortId, isValidTransactionHash, isValidUUID } from "../utils";
import { useTranslation } from "react-i18next";
import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import { TransactionOverview } from "../components/TransactionOverview";
import { EventsOverview } from "../components/EventsOverview";
import { PaladinTransactionSection } from "../components/PaladinTransactionSection";
import { ReceiptlessPaladinTransaction } from "../components/ReceiptlessPaladinTransaction";
import { PaladinTransactionsDetails } from "../components/TransactionDetails";

export const TransactionEntry: React.FC = () => {

  const navigate = useNavigate();
  const { t } = useTranslation();
  const { hashOrId } = useParams();
  const [searchParams] = useSearchParams();
  const [hash, setHash] = useState<string>();
  const [id, setId] = useState<string>();

  useEffect(() => {
    if (hashOrId === undefined) {
      navigate('/ui/transactions');
    } else if (isValidTransactionHash(hashOrId)) {
      setHash(hashOrId);
    } else if (isValidUUID(hashOrId)) {
      setId(hashOrId);
    } else {
      navigate('/ui/transactions');
    }
  }, [hashOrId]);

  const { data: enrichedTransaction, error: blockchainTransactionError } = useQuery({
    queryKey: [`blockchain-transaction-${hash}`],
    queryFn: () => fetchEnrichedTransaction(hash!),
    enabled: hash !== undefined
  });

  const { data: receipt, error: receiptError } = useQuery({
    queryKey: [`paladin-receipt-${id}`],
    queryFn: () => fetchTransactionReceipt(id!),
    enabled: id !== undefined
  });

  const { data: paladinTransaction, error: paladinTransactionError } = useQuery({
    queryKey: [`paladin-transaction-${id}`],
    queryFn: () => fetchPaladinTransaction(id!),
    enabled: id !== undefined && receipt === null
  });

  useEffect(() => {
    if (hash === undefined) {
      if (receipt !== undefined && receipt !== null) {
        setHash(receipt.transactionHash);
      }
    }
  }, [hash, receipt]);

  if (hash === undefined && id === undefined) {
    return <></>;
  }

  if (blockchainTransactionError || paladinTransactionError || receiptError) {
    return <Alert sx={{ margin: '30px' }} severity="error" variant="filled">{blockchainTransactionError?.message ?? paladinTransactionError?.message}</Alert>
  }

  const back = searchParams.get('back');
  const backTo = (back !== null && ['submissions', 'domains'].includes(back)) ? back : 'transactions';

  return (
    <Fade timeout={600} in={true}>
      <Box
        sx={{
          padding: "20px",
          maxWidth: "1500px",
          marginLeft: "auto",
          marginRight: "auto",
        }}
      >
        <Box sx={{ marginBottom: '20px' }}>
          <Button
            startIcon={<ArrowBackIcon fontSize="small" />}
            onClick={() => navigate(`/ui/${backTo}`)}
            >
            {t(`backTo${capitalize(backTo)}`)}
          </Button>
        </Box>
        {enrichedTransaction !== undefined &&
          <Grid2 container spacing={3}>
            <Grid2 size={{ xs: 12, sm: 12, md: 4, lg: 4 }}>
              <Box>
                <Typography variant="h6" sx={{ marginBottom: '5px' }}>{t('blockchainTransaction')}</Typography>
                <TransactionOverview
                  transaction={enrichedTransaction}
                />
                {enrichedTransaction.events.length > 0 &&
                  <>
                    <Typography variant="h6" sx={{ marginTop: '20px', marginBottom: '5px' }}>{t('events')}</Typography>
                    <EventsOverview
                      events={enrichedTransaction.events} />
                  </>}
              </Box>
            </Grid2>
            <Grid2 size={{ xs: 12, sm: 12, md: 8, lg: 8 }}>
              {enrichedTransaction.receipts.length > 0 &&
                <Box>
                  <Typography variant="h6" sx={{ marginBottom: '5px' }}>{t('paladinTransaction')}</Typography>
                  <PaladinTransactionSection receipts={enrichedTransaction.receipts} />
                </Box>}
            </Grid2>
          </Grid2>}
        {enrichedTransaction === undefined && receipt &&
          <>
            <Typography variant="h6" sx={{ marginBottom: '5px' }}>{t('paladinTransaction')}</Typography>
            <Tabs
              value="paladin-transaction"
              TabIndicatorProps={{ style: { display: 'none' } }}
            >
              <Tab value="paladin-transaction"
                sx={{
                  backgroundColor: theme => theme.palette.background.paper,
                  borderTopLeftRadius: '4px',
                  borderTopRightRadius: '4px'
                }}
                label={
                  <Box>
                    <span style={{ fontWeight: 600, marginRight: '6px' }}>{t(receipt.domain ?? 'public')}</span>
                    {getShortId(receipt.id)}
                  </Box>
                } />
            </Tabs>
            <PaladinTransactionsDetails receipt={receipt} />
          </>
        }
        {paladinTransaction !== undefined &&
          <ReceiptlessPaladinTransaction paladinTransaction={paladinTransaction} />
        }
      </Box>
    </Fade>
  );

}
