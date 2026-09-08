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
import { startEventListener, stopEventListener } from "../queries/transactions";
import { useTranslation } from "react-i18next";
import { IEventListener } from "../interfaces";
import { DeleteEventListenerDialog } from "../dialogs/DeleteEventListener";
import { useState } from "react";
import { useApplicationContext } from "../contexts/ApplicationContext";
import { ActionButton } from "./ActionButton";

type Props = {
  eventListener: IEventListener
  refetch: () => any
  deleteRefetch?: () => any
};

export const EventListenerActions: React.FC<Props> = ({
  eventListener,
  refetch,
  deleteRefetch
}) => {

  const { readOnly } = useApplicationContext();
  const [deleteEventListenerDialogOpen, setDeleteEventListenerDialogOpen] = useState(false);
  const { t } = useTranslation();

  const { mutate: startListener } = useMutation({
    mutationFn: (listenerName: string) => startEventListener(listenerName),
    onSuccess: () => refetch()
  });

  const { mutate: stopListener } = useMutation({
    mutationFn: (listenerName: string) => stopEventListener(listenerName),
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
          disabled={eventListener.started === true}
          onClick={() => {
            startListener(eventListener.name)
          }}
        >
          {t('start')}
        </ActionButton>
        <ActionButton
          disabled={eventListener.started !== true}
          onClick={() => {
            stopListener(eventListener.name)
          }}
        >{t('stop')}
        </ActionButton>
        <ActionButton
          color="error"
          onClick={() => {
            setDeleteEventListenerDialogOpen(true);
          }}
        >{t('delete')}
        </ActionButton>
      </Box>
      {deleteEventListenerDialogOpen && (
        <DeleteEventListenerDialog
          listenerName={eventListener.name}
          refetch={deleteRefetch ?? refetch}
          onClose={() => setDeleteEventListenerDialogOpen(false)}
        />
      )}
    </>
  )

}
