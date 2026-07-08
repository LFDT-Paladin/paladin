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

import { Accordion, AccordionDetails, AccordionSummary, Alert, Box, Button, Fade, Tab, Tabs, Typography } from "@mui/material";
import { useQuery } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router-dom";
import { getShortId } from "../utils";
import { useTranslation } from "react-i18next";
import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import { JSONBox } from "../components/JSONBox";
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import { getPrivacyGroupMessage } from "../queries/privacyGroups";

export const PrivateGroupMessageEntry: React.FC = () => {

  const navigate = useNavigate();
  const { t } = useTranslation();
  const { messageId } = useParams();

  const { data: privateGroupMessage, error } = useQuery({
    queryKey: ['private-group-message', messageId],
    queryFn: () => getPrivacyGroupMessage(messageId!),
    enabled: messageId !== undefined
  });

  if (privateGroupMessage === undefined) {
    return <></>;
  }

  if (privateGroupMessage === null) {
    return <Alert sx={{ margin: '30px' }} severity="error" variant="filled">{t('messageNotFound')}</Alert>
  }

  if (error) {
    return <Alert sx={{ margin: '30px' }} severity="error" variant="filled">{error?.message}</Alert>
  }

  return (
    <Fade timeout={600} in={true}>
      <Box
        sx={{
          padding: "20px",
          maxWidth: "1500px",
          marginLeft: "auto",
          marginRight: "auto",
        }}
      >
        <Box sx={{ marginBottom: '20px' }}>
          <Button
            startIcon={<ArrowBackIcon fontSize="small" />}
            onClick={() => navigate('/ui/privacy-groups/messages')}
          >
            {t('backToMessages')}
          </Button>
        </Box>
        <Typography variant="h6" sx={{ marginBottom: '15px' }}>{t('privacyGroupMessage')}</Typography>
        <Tabs value="contract"
          TabIndicatorProps={{ style: { display: 'none' } }}
        >
          <Tab value="contract"
            sx={{
              backgroundColor: theme => theme.palette.background.paper,
              borderTopLeftRadius: '4px',
              borderTopRightRadius: '4px'
            }}
            label={
              <Box>
                {getShortId(privateGroupMessage.id)}
              </Box>
            } />
        </Tabs>
        <Box sx={{
          paddingLeft: '5px',
          paddingTop: '15px',
          paddingBottom: '5px',
          backgroundColor: theme => theme.palette.background.paper,
        }}>
        </Box>
        <Accordion elevation={0} disableGutters defaultExpanded>
          <AccordionSummary expandIcon={<ExpandMoreIcon />}>
            {t('details')}
          </AccordionSummary>
          <AccordionDetails >
            <JSONBox data={privateGroupMessage} />
          </AccordionDetails>
        </Accordion>
      </Box>
    </Fade>
  );
}
