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

import { Button } from '@mui/material';
import daysjs from 'dayjs';
import relativeTime from 'dayjs/plugin/relativeTime';
import { useState } from 'react';
import { TimestampDialog } from '../dialogs/Timestamp';

daysjs.extend(relativeTime);

type Props = {
  timestamp: string
};

export const Timestamp: React.FC<Props> = ({
  timestamp,
}) => {

  const [timestampDialogOpen, setTimestampDialogOpen] = useState(false);

  return (
    <>
      <Button
        sx={{ fontWeight: '400', whiteSpace: 'nowrap' }}
        size="small"
        onClick={() => setTimestampDialogOpen(true)}>
        {new Date(timestamp).toLocaleString()}
      </Button>
      <TimestampDialog
        timestamp={timestamp}
        dialogOpen={timestampDialogOpen}
        setDialogOpen={setTimestampDialogOpen}
      />
    </>
  );
}
