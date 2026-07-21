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

import { Box } from "@mui/material"
import { useMutation } from "@tanstack/react-query";
import { startReceiptListener, stopReceiptListener } from "../queries/transactions";
import { useTranslation } from "react-i18next";
import { IReceiptListener } from "../interfaces";
import { DeleteReceiptListenerDialog } from "../dialogs/DeleteReceiptListener";
import { useState } from "react";
import { useApplicationContext } from "../contexts/ApplicationContext";
import { ActionButton } from "./ActionButton";

type Props = {
  receiptListener: IReceiptListener
  refetch: () => any
  deleteRefetch?: () => any
};

export const ReceiptListenerActions: React.FC<Props> = ({
  receiptListener,
  refetch,
  deleteRefetch
}) => {

  const { readOnly } = useApplicationContext();
  const [deleteReceiptListenerDialogOpen, setDeleteReceiptListenerDialogOpen] = useState(false);
  const { t } = useTranslation();

  const { mutate: startListener } = useMutation({
    mutationFn: (listenerName: string) => startReceiptListener(listenerName),
    onSuccess: () => refetch()
  });

  const { mutate: stopListener } = useMutation({
    mutationFn: (listenerName: string) => stopReceiptListener(listenerName),
    onSuccess: () => refetch()
  });

  if(readOnly) {
    return <></>;
  }

  return (
    <>
      <Box sx={{
        display: 'flex',
        gap: '10px'
      }}>
        <ActionButton
          disabled={receiptListener.started === true}
          onClick={() => {
            startListener(receiptListener.name)
          }}
        >
          {t('start')}
        </ActionButton>
        <ActionButton
          disabled={receiptListener.started !== true}
          onClick={() => {
            stopListener(receiptListener.name)
          }}
        >{t('stop')}
        </ActionButton>
        <ActionButton
          color="error"
          onClick={() => {
            setDeleteReceiptListenerDialogOpen(true);
          }}
        >{t('delete')}
        </ActionButton>
      </Box>
      <DeleteReceiptListenerDialog
        listenerName={receiptListener.name}
        refetch={deleteRefetch ?? refetch}
        dialogOpen={deleteReceiptListenerDialogOpen}
        setDialogOpen={setDeleteReceiptListenerDialogOpen}
      />
    </>
  )

}
