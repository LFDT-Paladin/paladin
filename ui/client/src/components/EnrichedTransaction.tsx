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

import { Box, Chip, Grid2, Typography, useTheme } from "@mui/material";
import { IEnrichedTransaction } from "../interfaces"
import { useTranslation } from "react-i18next";
import { Hash } from "./Hash";
import { Captions } from "lucide-react";
import { TransactionStatus } from "./TransactionStatus";
import { EllapsedTime } from "./EllapsedTime";
import iconLight from '../../public/paladin-icon-light.svg';
import { PaladinTransactionChip } from "./PaladinTransactionChip";
import DnsIcon from '@mui/icons-material/Dns';
import { EventChip } from "./EventChip";

type Props = {
  enrichedTransaction: IEnrichedTransaction
}

export const EnrichedTransaction: React.FC<Props> = ({
  enrichedTransaction
}) => {

  const theme = useTheme();
  const { t } = useTranslation();

  

  return (
    <>
      <Box
        sx={{
          backgroundColor: (theme) => theme.palette.background.paper,
          borderRadius: '4px',
          borderLeft: `solid 3px ${theme.palette.primary.main}`
        }}>
        <Box sx={{ padding: '10px' }}>
          <Grid2 container justifyContent="space-between" spacing={3}>
            <Grid2>
              <Typography align="center" variant="body2" color="textSecondary">{t('transactionHash')}</Typography>
              <Box sx={{ width: '140px' }}>
                <Hash Icon={<Captions size="18px" />} title={t('hash')} hash={enrichedTransaction.hash} hideTitle />
              </Box>
            </Grid2>
            <Grid2>
              <Typography align="center" variant="body2" color="textSecondary">{t('block')}</Typography>
              <Typography align="center" variant="h6" color="textPrimary">{enrichedTransaction.blockNumber.toLocaleString()}</Typography>
            </Grid2>
            <Grid2>
              <Typography align="center" variant="body2" color="textSecondary">{t('transactionIndex')}</Typography>
              <Typography align="center" variant="h6" color="textPrimary">{enrichedTransaction.transactionIndex}</Typography>
            </Grid2>
            <Grid2>
              <Typography align="center" variant="body2" color="textSecondary">{t('nonce')}</Typography>
              <Typography align="center" variant="h6" color="textPrimary">{enrichedTransaction.nonce}</Typography>
            </Grid2>
            <Grid2>
              <Typography align="center" variant="body2" color="textSecondary">{t('from')}</Typography>
              <Box sx={{ width: '140px' }}>
                <Hash Icon={<Captions size="18px" />} title={t('hash')} hash={enrichedTransaction.from} hideTitle />
              </Box>
            </Grid2>
            <Grid2>
              <Typography align="center" variant="body2" color="textSecondary">{t('to')}</Typography>
              <Box sx={{ width: '140px' }}>
                {enrichedTransaction.to ?
                  <Hash Icon={<Captions size="18px" />} title={t('to')} hash={enrichedTransaction.to} hideTitle />
                  :
                  <Typography align="center" variant="h6" color="textPrimary">--</Typography>
                }
              </Box>
            </Grid2>
            <Grid2>
              <Typography align="center" variant="body2" color="textSecondary">{t('contractAddress')}</Typography>
              <Box sx={{ width: '140px' }}>
                {enrichedTransaction.contractAddress ?
                  <Hash Icon={<Captions size="18px" />} title={t('hash')} hash={enrichedTransaction.contractAddress} hideTitle />
                  :
                  <Typography align="center" variant="h6" color="textPrimary">--</Typography>
                }
              </Box>
            </Grid2>
            <Grid2>
              <Typography align="center" variant="body2" color="textSecondary">{t('status')}</Typography>
              <TransactionStatus transaction={enrichedTransaction} />
            </Grid2>
            <Grid2>
              <Typography align="center" variant="body2" color="textSecondary">{t('time')}</Typography>
              <EllapsedTime icon={<></>} timestamp={enrichedTransaction.block.timestamp} />
            </Grid2>
          </Grid2>
        </Box>
        <Box
          sx={{
            borderTop: 'solid 1px',
            borderColor: theme => theme.palette.divider,
            overflowX: 'scroll',
          }}>
          <Box sx={{
            display: 'flex',
            gap: '8px',
            alignItems: 'center',
            padding: '10px'

          }}>
            <img src={iconLight} width="12" />
            <Typography variant="body2">{t('paladin')}</Typography>
            <Chip label={enrichedTransaction.paladinTransactions.length} sx={{ borderRadius: '4px', height: '25px' }} />
            <Box sx={{ width: '20px' }} />
            {enrichedTransaction.paladinTransactions.map(paladinTransaction =>
              <PaladinTransactionChip 
              key={paladinTransaction.id} 
              paladinTransaction={paladinTransaction}
              blockchainTransactionHash={enrichedTransaction.hash}
               />
            )}
          </Box>
        </Box>
        <Box
          sx={{
            borderTop: 'solid 1px',
            borderColor: theme => theme.palette.divider,
            overflowX: 'scroll',
          }}>
          <Box sx={{
            display: 'flex',
            gap: '8px',
            alignItems: 'center',
            padding: '10px'

          }}>
            <DnsIcon sx={{ fontSize: '16px' }} />
            <Typography variant="body2">{t('events')}</Typography>
            <Chip label={enrichedTransaction.events.length} sx={{ borderRadius: '4px', height: '25px' }} />
            <Box sx={{ width: '20px' }} />
            {enrichedTransaction.events.sort((a, b) => a.logIndex - b.logIndex).map(event =>
              <EventChip key={`${event.transactionHash}-${event.logIndex}`} event={event} />
            )}
          </Box>
        </Box>
      </Box>
    </>
  );

}