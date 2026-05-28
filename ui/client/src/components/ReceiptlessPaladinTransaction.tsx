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

import { Accordion, AccordionDetails, AccordionSummary, Box, Tab, Tabs, Typography } from "@mui/material"
import { IPaladinTransaction } from "../interfaces"
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import { useTranslation } from "react-i18next";
import { JSONBox } from "./JSONBox";
import { getShortId } from "../utils";

type Props = {
  paladinTransaction: IPaladinTransaction
}

export const ReceiptlessPaladinTransaction: React.FC<Props> = ({ paladinTransaction }) => {
  const { t } = useTranslation();

  return (
    <>
      <Typography align="center" variant="h6" sx={{ marginBottom: '5px' }}>{t('paladinTransaction')}</Typography>
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
              <span style={{ fontWeight: 600, marginRight: '6px' }}>{t(paladinTransaction.domain ?? 'public')}</span>
              {getShortId(paladinTransaction.id)}
            </Box>
          } />
      </Tabs>
      <Accordion elevation={0} disableGutters defaultExpanded>
        <AccordionSummary expandIcon={<ExpandMoreIcon />}>
          {t('details')}
        </AccordionSummary>
        <AccordionDetails >
          <JSONBox data={paladinTransaction} />
        </AccordionDetails>
      </Accordion>
    </>
  )

}