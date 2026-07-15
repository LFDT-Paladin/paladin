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

import {
  Alert,
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  InputAdornment,
  List,
  ListItem,
  TextField,
  Tooltip,
  Typography
} from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { isValidPrivacyGroupMemberName, isValidPrivacyGroupName } from '../utils';
import { useNavigate } from 'react-router-dom';
import DeleteOutlineOutlinedIcon from '@mui/icons-material/DeleteOutlineOutlined';
import { useMutation } from '@tanstack/react-query';
import { createPrivacyGroup } from '../queries/privacyGroups';

type Props = {
  dialogOpen: boolean
  setDialogOpen: React.Dispatch<React.SetStateAction<boolean>>
}

export const CreatePrivacyGroupDialog: React.FC<Props> = ({
  dialogOpen,
  setDialogOpen,
}) => {

  const { t } = useTranslation();
  const [name, setName] = useState('');
  const [member, setMember] = useState('');
  const [members, setMembers] = useState<string[]>([]);
  const [errorMessage, setErrorMessage] = useState<string>();
  const [showMemberNameError, setShowMemberNameError] = useState(false);
  const navigate = useNavigate();

  useEffect(() => {
    if (dialogOpen) {
      setName('');
      setMember('');
      setMembers([]);
      setShowMemberNameError(false);
      setErrorMessage(undefined);
    }
  }, [dialogOpen]);

  const { mutate: handleSubmit } = useMutation({
    mutationFn: () => createPrivacyGroup(name, members),
    onSuccess: data => {
      navigate(`/ui/privacy-groups/groups/${data.id}`);
    },
    onError: error => {
      setErrorMessage(error.message);
    }
  });

  const handleAddMemberName = () => {
    if (!isValidPrivacyGroupMemberName(member)) {
      setShowMemberNameError(true);
    } else {
      const updatedMembers = [...members];
      updatedMembers.push(member);
      setMembers(updatedMembers);
      setMember('');
      setShowMemberNameError(false);
    }
  };

  const canSubmit = isValidPrivacyGroupName(name) && members.length > 0;

  return (
    <Dialog
      onClose={() => setDialogOpen(false)}
      open={dialogOpen}
      fullWidth
      maxWidth="xs"
    >
      <form onSubmit={(event) => {
        event.preventDefault();
        handleSubmit();
      }}>
        <DialogTitle>
          {t('createPrivacyGroup')}
          {errorMessage && (
            <Alert variant="filled" severity="error">
              {errorMessage}
            </Alert>
          )}
        </DialogTitle>
        <DialogContent>
          <Box sx={{ marginTop: '6px' }}>

            <TextField
              label={t('name')}
              autoComplete="off"
              sx={{ marginBottom: '20px' }}
              fullWidth
              value={name}
              onChange={event => setName(event.target.value)}
              error={name.length > 0 && !isValidPrivacyGroupName(name)}
              helperText={(name.length > 0 && !isValidPrivacyGroupName(name)) ? t('privacyGroupNameRestrictions') : undefined}
            />
            <TextField
              label={t('memberName')}
              autoComplete="off"
              fullWidth
              value={member}
              onChange={event => setMember(event.target.value)}
              error={showMemberNameError}
              helperText={showMemberNameError ? t('memberNameRestrictions') : undefined}
              onKeyDown={event => {
                if (event.key === 'Enter') {
                  if (!members.includes(member)) {
                    handleAddMemberName();
                  }
                  event.preventDefault();
                }
              }}
              sx={{ marginBottom: '20px' }}
              slotProps={{
                input: {
                  endAdornment: (
                    <InputAdornment position="end">
                      <Button variant="contained"
                        disabled={member.length === 0 || members.includes(member)}
                        onClick={() => handleAddMemberName()}
                      >{t('add')}</Button>
                    </InputAdornment>
                  )
                }
              }}
            />
            <Typography variant="h6">{t('members')}</Typography>
            <Box sx={{
              border: 'solid 1px',
              borderColor: theme => theme.palette.action.disabled,
              borderRadius: '4px',
              minHeight: '180px'
            }}>
              <List>
                {members.map(member =>
                  <ListItem key={member} value={member}
                    secondaryAction={
                      <Tooltip title={t('removeMember')} arrow>
                        <IconButton edge="end"
                          onClick={() => {
                            const updatedMembers = members.filter(currentMember => currentMember !== member);
                            setMembers(updatedMembers);
                          }}>
                          <DeleteOutlineOutlinedIcon />
                        </IconButton>
                      </Tooltip>
                    }
                  >{member}</ListItem>
                )}
              </List>
            </Box>
          </Box>
        </DialogContent>
        <DialogActions sx={{ justifyContent: 'center', marginBottom: '15px' }}>
          <Button
            sx={{ minWidth: '100px' }}
            size="large"
            variant="contained"
            disableElevation
            disabled={!canSubmit}
            type="submit">
            {t('create')}
          </Button>
          <Button
            sx={{ minWidth: '100px' }}
            size="large"
            variant="outlined"
            disableElevation
            onClick={() => setDialogOpen(false)}
          >
            {t('cancel')}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
};
