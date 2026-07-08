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
import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { customNavigate, getShortId, isValidAddress, isValidPrivacyGroupId } from "../utils";
import { useTranslation } from "react-i18next";
import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import { getPrivacyGroupByAddress, getPrivacyGroupById } from "../queries/privacyGroups";
import { JSONBox } from "../components/JSONBox";
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import { useApplicationContext } from "../contexts/ApplicationContext";
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import { AppRoutes } from "../routes";

export const PrivacyGroupEntry: React.FC = () => {

  const navigate = useNavigate();
  const { t } = useTranslation();
  const { idOrAddress } = useParams();
  const [address, setAddress] = useState<string>();
  const [id, setId] = useState<string>();

  const { privateGroupMessages: privateGroupMessagesViewStateState } = useApplicationContext();
  const {
    setFilters,
    setFiltersVisible
  } = privateGroupMessagesViewStateState;

  useEffect(() => {
    if (idOrAddress === undefined) {
      navigate('/ui/privacy-groups');
    } else if (isValidAddress(idOrAddress)) {
      setAddress(idOrAddress);
    } else if (isValidPrivacyGroupId(idOrAddress)) {
      setId(idOrAddress);
    } else {
      navigate('/ui/privacy-groups');
    }
  }, [idOrAddress]);

  const { data: privacyGroupById, error: privacyGroupByIdError } = useQuery({
    queryKey: [`privacy-group-by-${id}`],
    queryFn: () => getPrivacyGroupById(id!),
    enabled: id !== undefined
  });

  const { data: privacyGroupByAddress, error: privacyGroupByAddressError } = useQuery({
    queryKey: [`privacy-group-by-address-${address}`],
    queryFn: () => getPrivacyGroupByAddress(address!),
    enabled: address !== undefined
  });

  const privacyGroup = privacyGroupById ?? privacyGroupByAddress;

  if (privacyGroup === undefined) {
    return <></>;
  }

  if (privacyGroupByIdError || privacyGroupByAddressError) {
    return <Alert sx={{ margin: '30px' }} severity="error" variant="filled">{privacyGroupByIdError?.message ?? privacyGroupByAddressError?.message}</Alert>
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
            onClick={() => navigate(AppRoutes.PrivacyGroups)}
          >
            {t('backToPrivacyGroups')}
          </Button>
        </Box>
        <Typography variant="h6" sx={{ marginBottom: '15px' }}>{t('privacyGroup')}</Typography>
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
                {privacyGroup.name} {getShortId(privacyGroup.id)}
              </Box>
            } />
        </Tabs>
        <Box sx={{
          paddingLeft: '5px',
          paddingTop: '15px',
          paddingBottom: '5px',
          backgroundColor: theme => theme.palette.background.paper,
        }}>
          <Button size="small"
            endIcon={<OpenInNewIcon />}
            sx={{ fontWeight: '400', minWidth: '70px' }}
            onClick={event => {
              setFilters([
                {
                  operator: 'equal',
                  value: privacyGroup.id,
                  field: {
                    label: t('group'),
                    name: 'group',
                    type: 'string',
                    isHexValue: true
                  }
                }
              ]);
              setFiltersVisible(true);
              customNavigate(AppRoutes.PrivacyGroupMessages, event, navigate)
            }}
          >{t('exploreGroupMessages')}</Button>
        </Box>
        <Accordion elevation={0} disableGutters defaultExpanded>
          <AccordionSummary expandIcon={<ExpandMoreIcon />}>
            {t('details')}
          </AccordionSummary>
          <AccordionDetails >
            <JSONBox data={privacyGroup} />
          </AccordionDetails>
        </Accordion>
      </Box>
    </Fade>
  );
}
