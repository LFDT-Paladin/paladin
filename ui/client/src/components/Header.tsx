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

import MenuIcon from '@mui/icons-material/Menu';
import {
  AppBar,
  Box,
  ButtonBase,
  IconButton,
  Toolbar,
  useTheme,
} from '@mui/material';
import { useState } from 'react';
import logoDark from '../../public/paladin-title-dark.svg';
import logoLight from '../../public/paladin-title-light.svg';
import { SettingsMenu } from '../menus/Settings';

interface Props {
  navigationVisible: boolean;
  setNavigationVisible: React.Dispatch<React.SetStateAction<boolean>>;
}

export const Header: React.FC<Props> = ({
  navigationVisible,
  setNavigationVisible
}) => {

  const theme = useTheme();
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);

  return (
    <>
      <AppBar
        elevation={0}
        sx={{
          zIndex: (theme) => theme.zIndex.drawer + 1
        }}>
        <Toolbar
          sx={{
            backgroundColor: (theme) => theme.palette.background.paper,
            minHeight: { xs: '60px' },
            paddingLeft: { xs: '10px'}
          }}
        >
          <IconButton
            onClick={() => setNavigationVisible(!navigationVisible)}
          >
            <MenuIcon />
          </IconButton>
          <ButtonBase
            onClick={() => window.location.href = '/ui'}>
            <img
              src={theme.palette.mode === 'dark' ? logoDark : logoLight}
            />
          </ButtonBase>
          
        </Toolbar>
      </AppBar>
      <Box
        sx={{
          height: theme => theme.mixins.toolbar
        }}
      />
      <SettingsMenu anchorEl={anchorEl} setAnchorEl={setAnchorEl} />
    </>
  );
};
