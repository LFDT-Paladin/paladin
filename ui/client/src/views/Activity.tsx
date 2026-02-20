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

import { useState, useEffect } from "react";
import { Box, Fade, Grid2, TextField } from "@mui/material";
import { Transactions } from "../components/Transactions";
import { Events } from "../components/Events";
import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router-dom";

export const Activity: React.FC = () => {

  const { t } = useTranslation();
  const [txHash, setTxHash] = useState('');

  const [searchParams] = useSearchParams();
  
  useEffect(() => {
    if (searchParams.get('hash')) {
      setTxHash(searchParams.get('hash') as string);
    }
  }, [searchParams]);

  return (
    <Fade timeout={600} in={true}>
      <Box sx={{ padding: '10px', paddingTop: '30px', maxWidth: '1300px', marginLeft: 'auto', marginRight: 'auto' }}>
        <Grid2 container marginBottom={4}>
            <TextField label={t("filterByTransactionHash")} fullWidth size="medium" value={txHash} onChange={(e) => setTxHash(e.target.value)} />
        </Grid2>
        <Grid2 container>
          <Grid2 size={{ md: 6, sm: 12, xs: 12 }} alignSelf="center" borderRight={2} borderColor={theme => theme.palette.primary.main} paddingRight={8}>
            <Transactions txHash={txHash} />
          </Grid2>
          <Grid2 size={{ md: 6, sm: 12, xs: 12 }} paddingLeft={8}>
            <Events txHash={txHash} />
          </Grid2>
        </Grid2>
      </Box>
    </Fade>
  );
};
