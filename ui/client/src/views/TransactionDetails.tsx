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

import { Alert, Box, Button, Fade, Grid2, Typography } from "@mui/material";
import { useQuery } from "@tanstack/react-query";
import { fetchTransaction } from "../queries/transactions";
import { useEffect } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { isValidTransactionHash } from "../utils";
import { useTranslation } from "react-i18next";
import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import { TransactionOverview } from "../components/TransactionOverview";
import { EventsOverview } from "../components/EventsOverview";
import { PaladinTransactionSection } from "../components/PaladinTransactionSection";

export const TransactionDetails: React.FC = () => {

  const navigate = useNavigate();
  const { t } = useTranslation();
  const { hash } = useParams();

  useEffect(() => {
    if (hash === undefined || !(isValidTransactionHash(hash))) {
      navigate('/ui/transactions');
    }
  }, [hash]);

  if (hash === undefined) {
    return <></>;
  }

  const { data: enrichedTransaction, error } = useQuery({
    queryKey: [`transaction-${hash}`],
    queryFn: () => fetchTransaction(hash)
  });

  if (error) {
    return <Alert sx={{ margin: '30px' }} severity="error" variant="filled">{error.message}</Alert>
  }

  if (enrichedTransaction === undefined) {
    return <></>;
  }

  return (
    <Fade timeout={600} in={true}>
      <Box
        sx={{
          padding: "20px",
          maxWidth: "1300px",
          marginLeft: "auto",
          marginRight: "auto",
        }}
      >
        <Box sx={{ marginBottom: '20px' }}>
          <Button startIcon={<ArrowBackIcon fontSize="small" />}
            onClick={() => navigate('/ui/transactions')}>
            {t('backToTransactions')}
          </Button>
        </Box>
        <Grid2 container spacing={3}>
          <Grid2 size={{ xs: 12, sm: 12, md: 4, lg: 3 }}>
            <Box>
              <Typography align="center" variant="h6" sx={{ marginBottom: '5px' }}>{t('blockchainTransaction')}</Typography>
              <TransactionOverview
                transaction={enrichedTransaction}
              />
              <Typography align="center" variant="h6" sx={{ marginTop: '20px', marginBottom: '5px' }}>{t('events')}</Typography>
              <EventsOverview
                events={enrichedTransaction.events} />
            </Box>
          </Grid2>
          <Grid2 size={{ xs: 12, sm: 12, md: 8, lg: 9 }}>
            <Box>
              <Typography align="center" variant="h6" sx={{ marginBottom: '5px' }}>{t('paladinTransaction')}</Typography>
              <PaladinTransactionSection paladinTransactions={enrichedTransaction.paladinTransactions} />
            </Box>
          </Grid2>
        </Grid2>
      </Box>
    </Fade>
  );

}
