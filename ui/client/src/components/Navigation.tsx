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

import { Box, ButtonBase, Drawer, IconButton, List, ListItem, ListItemButton, ListItemText, useTheme } from "@mui/material";
import { useLocation, useNavigate, useSearchParams } from "react-router-dom";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useApplicationContext } from "../contexts/ApplicationContext";
import { CONSTANTS, customNavigate } from "../utils";
import { AppRoutes } from "../routes";
import logoDark from '../../public/paladin-title-dark.svg';
import logoLight from '../../public/paladin-title-light.svg';
import { SettingsMenu } from "../menus/Settings";
import SettingsIcon from '@mui/icons-material/Settings';

export const Navigation: React.FC = () => {
  const { navigationVisible, setNavigationVisible } = useApplicationContext();
  const navigate = useNavigate();
  const pathname = useLocation().pathname.toLowerCase();
  const { t } = useTranslation();
  const theme = useTheme();
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);

  const [searchParams] = useSearchParams();
  const back = searchParams.get('back') ?? '';

  const drawerContent = (
    <>
      <ButtonBase
        sx={{
          marginTop: '20px',
          marginBottom: '10px'
        }}
        onClick={() => window.location.href = '/ui'}>
        <img
          src={theme.palette.mode === 'dark' ? logoDark : logoLight}
        />
      </ButtonBase>
      <List className="mainNavigation">
        <ListItem>
          <ListItemButton
            onClick={event => customNavigate(AppRoutes.Transactions, event, navigate)}
            selected={pathname.startsWith(AppRoutes.Transactions) && !['submissions'].includes(back) }>
            <ListItemText primary={t('transactions')} />
          </ListItemButton>
        </ListItem>
        <ListItem>
          <ListItemButton
            onClick={event => customNavigate(AppRoutes.Submissions, event, navigate)}
            selected={pathname.startsWith(AppRoutes.Submissions) || back === 'submissions' }>
            <ListItemText primary={t('submissions')} />
          </ListItemButton>
        </ListItem>
        <ListItem>
          <ListItemButton
            onClick={event => customNavigate(AppRoutes.Keys, event, navigate)}
            selected={pathname.startsWith(AppRoutes.Keys)}>
            <ListItemText primary={t('keys')} />
          </ListItemButton>
        </ListItem>
        <ListItem>
          <ListItemButton
            onClick={event => customNavigate(AppRoutes.Registries, event, navigate)}
            selected={pathname.startsWith(AppRoutes.Registries)}>
            <ListItemText primary={t('registries')} />
          </ListItemButton>
        </ListItem>
        <ListItem>
          <ListItemButton
            onClick={event => customNavigate(AppRoutes.Domains, event, navigate)}
            selected={pathname.startsWith(AppRoutes.Domains)}>
            <ListItemText primary={t('domains')} />
          </ListItemButton>
        </ListItem>
        <ListItem>
          <ListItemButton
            onClick={event => customNavigate(AppRoutes.PrivacyGroups, event, navigate)}
            selected={pathname.startsWith('/ui/privacy-groups')}>
            <ListItemText primary={t('privacyGroups')} />
          </ListItemButton>
        </ListItem>
        <ListItem>
          <ListItemButton
            onClick={event => customNavigate(AppRoutes.States, event, navigate)}
            selected={pathname.startsWith(AppRoutes.States)}>
            <ListItemText primary={t('states')} />
          </ListItemButton>
        </ListItem>
        <ListItem>
          <ListItemButton
            onClick={event => customNavigate(AppRoutes.TransportConnections, event, navigate)}
            selected={pathname.startsWith('/ui/transports')}>
            <ListItemText primary={t('transports')} />
          </ListItemButton>
        </ListItem>
        <ListItem>
          <ListItemButton
            onClick={event => customNavigate(AppRoutes.EventListeners, event, navigate)}
            selected={pathname.startsWith('/ui/listeners')}>
            <ListItemText primary={t('listeners')} />
          </ListItemButton>
        </ListItem>
      </List>
      <Box sx={{ display: 'flex', height: '100%', padding: '5px' }}>
        <IconButton
          id="settings"
          size="large"
          sx={{
            marginTop: 'auto'
          }}
          onClick={(event) => setAnchorEl(event.currentTarget)}
        >
          <SettingsIcon />
        </IconButton>
      </Box>
      <SettingsMenu anchorEl={anchorEl} setAnchorEl={setAnchorEl} />

    </>
  );

  return (
    <>
      <Drawer
        variant="temporary"
        open={navigationVisible}
        sx={{
          display: { md: 'block', lg: 'none' },
          '& .MuiDrawer-paper': { width: CONSTANTS.NAVIGATION_DRAWER_WIDTH }
        }}
        PaperProps={{
          sx: {
            backgroundColor: theme.palette.mode === 'dark' ? theme => theme.palette.background.default : undefined
          }
        }}
        onClose={() => setNavigationVisible(false)}
      >
        {drawerContent}
      </Drawer>
      <Drawer
        PaperProps={{
          sx: {
            border: 'none'
          }
        }}
        variant="permanent"
        sx={{
          display: { lg: 'block', md: 'none', sm: 'none', xs: 'none' },
          width: CONSTANTS.NAVIGATION_DRAWER_WIDTH,
          '& .MuiDrawer-paper': { width: CONSTANTS.NAVIGATION_DRAWER_WIDTH },
        }}
      >
        {drawerContent}
      </Drawer>
    </>
  );

}
