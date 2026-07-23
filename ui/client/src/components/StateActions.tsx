// Copyright contributors to Paladin, an LFDT project
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

import { useTranslation } from "react-i18next";
import { IState } from "../interfaces";
import { Box } from "@mui/material";
import { SendStateDialog } from "../dialogs/SendState";
import { useState } from "react";
import { ActionButton } from "./ActionButton";

type Props = {
  state: IState
}

export const StateActions: React.FC<Props> = ({ state }) => {

  const [sendStateDialogOpen, setSendStateDialogOpen] = useState(false);
  const { t } = useTranslation();

  return (
    <>
      <Box sx={{ display: 'flex', gap: '20px' }}>
        <ActionButton
          onClick={() => setSendStateDialogOpen(true)}
        >
          {t('send')}
        </ActionButton>
      </Box>
      {sendStateDialogOpen && (
        <SendStateDialog
          state={state}
          onClose={() => setSendStateDialogOpen(false)}
        />
      )}
    </>
  );
}
