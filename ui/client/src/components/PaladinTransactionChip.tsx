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

import { Button } from "@mui/material";
import { IPaladinTransaction } from "../interfaces";
import { getShortId } from "../utils";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import OpenInNewIcon from '@mui/icons-material/OpenInNew';

type Props = {
  blockchainTransactionHash: string
  paladinTransaction: IPaladinTransaction
}

export const PaladinTransactionChip: React.FC<Props> = ({
  paladinTransaction,
  blockchainTransactionHash
}) => {

  const { t } = useTranslation();
  const navigate = useNavigate();

  return (
    <Button variant="contained" size="small" disableElevation
    color="info"
      sx={{
        paddingTop: 0, paddingBottom: 0, fontWeight: '400', whiteSpace: 'nowrap',
        minWidth: '155px'
      }}
      onClick={() => navigate(`/ui/transactions/${blockchainTransactionHash}/${paladinTransaction.id}`)}
      endIcon={<OpenInNewIcon />}
    >
      {paladinTransaction.domain !== undefined &&
        <span style={{ fontWeight: 600, marginRight: '6px' }}>{t(paladinTransaction.domain)}</span>}
      {getShortId(paladinTransaction.id)}
    </Button>
  );

};