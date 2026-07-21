// Copyright © 2024 Kaleido, Inc.
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
  Grid2 as Grid
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { SingleValue } from '../components/SingleValue';

type Props = {
  timestamp: string
  onClose: () => void
}

export const TimestampDialog: React.FC<Props> = ({
  timestamp,
  onClose
}) => {

  const { t } = useTranslation();
  const date = new Date(timestamp);

  const getEpoch = () => {
    let epoch = date.getTime().toString();
    const length = timestamp.length;
    switch (length) {
      case 30:
        return t('numberNanoseconds', { number: epoch + timestamp.substring(23, 29) });
      case 25:
        return t('numberMilliseconds', { number: timestamp.substring(18, 24) });
      default:
        return t('numberSeconds', { number: epoch.substring(0, 10) });
    }
  };

  return (
    <Dialog
      onClose={onClose}
      open
      fullWidth
      maxWidth="sm"
    >
      <DialogTitle sx={{ textAlign: 'center' }}>
        {t('timestamp')}
      </DialogTitle>
      <DialogContent>
        <Box sx={{ alignItems: 'center', minWidth: '300px', paddingTop: '5px' }}>
          <Grid container direction="column" spacing={2}>
            <Grid>
              <SingleValue label={t('localTime')} value={date.toLocaleString()} />
            </Grid>
            <Grid>
              <SingleValue label={t('ISO')} value={timestamp} />
            </Grid>
            <Grid>
              <SingleValue label={t('UTC')} value={date.toUTCString()} />
            </Grid>
            <Grid>
              <SingleValue label={t('epoch')} value={getEpoch()} />
            </Grid>
          </Grid>
        </Box>
      </DialogContent>
      <DialogActions sx={{ justifyContent: 'center', marginBottom: '15px' }}>
        <Button
          onClick={() => onClose()}
          variant="contained"
          disableElevation>
          {t('close')}
        </Button>
      </DialogActions>
    </Dialog>
  );
};
