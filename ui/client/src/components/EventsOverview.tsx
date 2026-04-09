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

import { Box, Paper, Table, TableBody, TableCell, TableRow, Typography } from "@mui/material";
import { IEvent } from "../interfaces";
import { useTranslation } from "react-i18next";
import { Hash } from "./Hash";
import { Captions } from "lucide-react";

type Props = {
  events: IEvent[]
}

export const EventsOverview: React.FC<Props> = ({ events }) => {

  const { t } = useTranslation();

  return (
    <Paper elevation={0}
    >
      <Table>
        <TableBody
          sx={{
            "& .MuiTableRow-root:last-child td, & .MuiTableRow-root:last-child th": {
              borderBottom: "none",
            },
          }}>

            {events.map(event =>
          <TableRow key={`${event.logIndex}-${event.signature}`} sx={{ height: '40px' }}>
            <TableCell width={'140px'}>
              <Typography variant="body2" color="textSecondary">{t('logIndexN', { n: event.logIndex})}</Typography>
            </TableCell>
            <TableCell>
              <Box sx={{ maxWidth: '140px' }}>
                <Hash Icon={<Captions size="18px" />} title={t('signature')} hash={event.signature} hideTitle />
              </Box>
            </TableCell>
          </TableRow>

            )}


        </TableBody>
      </Table>
    </Paper>
  );

};