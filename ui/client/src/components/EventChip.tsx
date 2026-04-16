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
import { IEvent } from "../interfaces";
import { getShortId } from "../utils";
import { useTranslation } from "react-i18next";
import { HashDialog } from "../dialogs/Hash";
import { useState } from "react";

type Props = {
  event: IEvent
}

export const EventChip: React.FC<Props> = ({ event }) => {

  const [hashDialogOpen, setHashDialogOpen] = useState(false);
  const { t } = useTranslation();

  return (
    <>
    <Button variant="contained" size="small" disableElevation
      sx={{
        paddingTop: 0, paddingBottom: 0, fontWeight: '400', whiteSpace: 'nowrap',
        minWidth: '155px'
      }}
      onClick={() => setHashDialogOpen(true)}
    >
      <span style={{ fontWeight: 600, marginRight: '6px' }}>{t('logN', { n: event.logIndex })}</span>
      {getShortId(event.signature)}
    </Button>
      <HashDialog dialogOpen={hashDialogOpen} setDialogOpen={setHashDialogOpen} title={t('signature')} hash={event.signature} />

    </>
  );

};