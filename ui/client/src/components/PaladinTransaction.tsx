// Copyright © 2025 Kaleido, Inc.
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

import { Box, Grid2, IconButton, Tooltip, Typography } from "@mui/material";
import { IPaladinTransaction } from "../interfaces";
import { Hash } from "./Hash";
import daysjs from 'dayjs';
import relativeTime from 'dayjs/plugin/relativeTime';
import { EllapsedTime } from "./EllapsedTime";
import { Captions, Tag } from 'lucide-react';
import { customNavigate } from "../utils";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import OpenInNewIcon from '@mui/icons-material/OpenInNew';

daysjs.extend(relativeTime);

type Props = {
  paladinTransaction: IPaladinTransaction;
};

export const PaladinTransaction: React.FC<Props> = ({ paladinTransaction }) => {

  const navigate = useNavigate();
  const { t } = useTranslation();

  if (paladinTransaction === undefined) {
    return <></>;
  }

  return (
    <>
      <Box
        sx={{
          backgroundColor: (theme) => theme.palette.background.paper,
          marginBottom: "20px",
          borderRadius: "4px"
        }}
      >
        <Box sx={{ padding: '10px', paddingLeft: '20px', paddingRight: '20px' }}>
          <Grid2 container justifyContent="space-between" alignItems="center" spacing={2}>
            <Grid2 textAlign="center" size={{ xs: 12, md: 2.5 }}>
              <Hash Icon={<Tag size="18px" />} title={t("id")} hash={paladinTransaction.id} />
            </Grid2>
            <Grid2 textAlign="center" size={{ xs: 12, md: 2.5 }}>
              <Hash Icon={<Captions size="18px" />} hash={paladinTransaction.from} title={t('from')} />
            </Grid2>
            <Grid2 textAlign="center" size={{ xs: 12, md: 2.5 }}>
              <Hash Icon={<Captions size="18px" />} hash={paladinTransaction.to ?? '--'} title={t('to')} />
            </Grid2>
            <Grid2 size={{ md: 1.25 }}>
              <Typography align="center" variant="h6">
                {t(paladinTransaction.type)}
              </Typography>
              <Typography align="center" variant="body2" color="textSecondary">
                {t("type")}
              </Typography>
            </Grid2>
            <Grid2 size={{ md: 1.25 }}>
              <Typography align="center" variant="h6" color="textPrimary">
                {paladinTransaction.domain ?? '--'}
              </Typography>
              <Typography align="center" variant="body2" color="textSecondary">
                {t("domain")}
              </Typography>
            </Grid2>
            <Grid2 size={{ md: 1 }}>
              <Box sx={{ minWidth: '100px', textAlign: 'center' }}>
                <EllapsedTime icon={null} timestamp={paladinTransaction.created} />
                <Typography align="center" variant="body2" color="textSecondary">{t('time')}</Typography>
              </Box>
            </Grid2>
            <Grid2 size={{ md: 1 }} sx={{ textAlign: 'right'}}>
              <Tooltip arrow title={t('open')}>
                <IconButton
                  onClick={event => customNavigate(`/ui/transactions/${paladinTransaction.id}?back=submissions`, event, navigate)}
                >
                  <OpenInNewIcon color="secondary" fontSize="medium" />
                </IconButton>
              </Tooltip>
            </Grid2>
          </Grid2>
        </Box>
      </Box>
    </>
  );
};
