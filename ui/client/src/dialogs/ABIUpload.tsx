// Copyright © 2024 Kaleido, Inc.
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
  FormControlLabel,
  Grid2 as Grid,
  Radio,
  RadioGroup,
  TextField,
  Typography
} from '@mui/material';
import { FileUploader } from 'react-drag-drop-files';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { UploadFile } from '@mui/icons-material';
import { useMutation } from '@tanstack/react-query';
import { uploadABI } from '../queries/storeABI';

type Props = {
  onClose: () => void
}

export const ABIUploadDialog: React.FC<Props> = ({
  onClose
}) => {

  const { t } = useTranslation();
  const [errorMessage, setErrorMessage] = useState<string>();
  const [fileSelected, setFileSelected] = useState<File | null>(null);
  const [radioSelection, setRadioSelection] = useState<'file' | 'text'>('file');
  const [abiText, setAbiText] = useState('');
  const [abiUploadCount, setAbiUploadCount] = useState(0);

  const { mutate, data, reset, error } = useMutation({
    mutationFn: (value: Object) => uploadABI(value)
  });

  useEffect(() => {
    if(error !== null) {
      setErrorMessage(t('invalidABI'));
    }
  }, [error]);

  const handleSubmit = async () => {
    setErrorMessage(undefined);
    reset();
    let valueToParse: string;
    let parsedValue: Object;
    if (radioSelection === 'file' && fileSelected !== null) {
      valueToParse = await fileSelected.text();
    } else {
      valueToParse = abiText;
    }
    try {
      parsedValue = JSON.parse(valueToParse);
      mutate(parsedValue);
      setAbiUploadCount(abiUploadCount + 1);
    } catch (err) {
      if (err !== undefined) {
        setErrorMessage(t('invalidABI'));
        return;
      }
    }
  };

  const canSubmit = radioSelection === 'file' && fileSelected !== null
    || radioSelection === 'text' && abiText.length > 0;

  return (
    <Dialog
      open
      fullWidth
      maxWidth="md"
      onClose={onClose}
    >
      <form onSubmit={(event) => {
        event.preventDefault();
        handleSubmit();
      }}>
        <DialogTitle sx={{ textAlign: 'center' }}>
          {t('uploadABI')}
          <Box sx={{ marginTop: '10px' }}>
            {errorMessage !== undefined &&
              <Alert variant="filled" severity="error">
                {errorMessage}
              </Alert>
            }
            {data !== undefined &&
              <Alert variant="filled" severity="success">
                {t('abiHash', { hash: data })}
              </Alert>
            }
          </Box>
        </DialogTitle>
        <DialogContent>

          <RadioGroup
            value={radioSelection}
            onChange={event => setRadioSelection(event.target.value as 'file' || 'text')}
          >
            <Grid container direction="column">
              <Grid>
                <FormControlLabel value="file" control={<Radio />} label={t('uploadFile')} />
              </Grid>
              <Grid>
                <FileUploader
                  disabled={radioSelection !== 'file'}
                  handleChange={(file: any) => {
                    setFileSelected(file);
                  }}
                  hoverTitle={t('dropFileHere')}
                  children={
                    <Box sx={{
                      display: 'flex',
                      height: '100px',
                      width: '100%',
                      padding: "10px",
                      borderRadius: '4px',
                      borderStyle: 'dashed',
                      cursor: radioSelection === 'file' ? 'pointer' : undefined,
                      alignItems: 'center',
                      justifyContent: 'center',
                      opacity: radioSelection !== 'file' ? '.4' : undefined
                    }}>
                      <UploadFile color="primary" />
                      <Typography align="center" fontWeight={500} sx={{ marginLeft: '4px' }}>
                        {fileSelected === null
                          ? t('uploadABIFileDescription')
                          : t('abiFileSelected', { fileName: fileSelected.name })}
                      </Typography>
                    </Box>
                  }
                  types={['abi']}
                />
              </Grid>
              <Grid>
                <Box sx={{ height: '25px' }} />
              </Grid>
              <Grid>
                <FormControlLabel value="text" control={<Radio />} label={t('pasteABI')} />
              </Grid>
              <Grid>
                <TextField
                  disabled={radioSelection !== 'text'}
                  fullWidth
                  multiline
                  rows={8}
                  value={abiText}
                  onChange={event => setAbiText(event.target.value)}
                />
              </Grid>
            </Grid>
          </RadioGroup>
        </DialogContent>
        <DialogActions sx={{ justifyContent: 'center', paddingBottom: '20px' }}>
          <Button
            sx={{ minWidth: '100px' }}
            size="large"
            variant="contained"
            disableElevation
            disabled={!canSubmit}
            type="submit">
            {t('upload')}
          </Button>
          <Button
            sx={{ minWidth: '100px' }}
            size="large"
            variant="outlined"
            disableElevation
            onClick={() => onClose()}
          >
            {t(abiUploadCount === 0 ? 'cancel' : 'close')}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
};
