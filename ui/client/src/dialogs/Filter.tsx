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

import {
  Box,
  Button,
  Checkbox,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  Grid2,
  MenuItem,
  TextField
} from '@mui/material';
import { useEffect, useState, type ReactElement } from 'react';
import { useTranslation } from 'react-i18next';
import type { Dayjs } from "dayjs";
import dayjs from 'dayjs';
import { AdapterDayjs } from '@mui/x-date-pickers/AdapterDayjs';
import { LocalizationProvider } from '@mui/x-date-pickers/LocalizationProvider';
import { DateTimePicker } from '@mui/x-date-pickers/DateTimePicker';
import { IFilter, IFilterField } from '../interfaces';
import { isValidHex, isValidUUID } from '../utils';

type Props = {
  existingFilter?: IFilter
  filterFields: IFilterField[]
  addFilter: (filter: IFilter) => void
  updateFilters: () => void
  dialogOpen: boolean
  setDialogOpen: React.Dispatch<React.SetStateAction<boolean>>
}

export const FilterDialog: React.FC<Props> = ({
  existingFilter,
  filterFields,
  addFilter,
  updateFilters,
  dialogOpen,
  setDialogOpen
}) => {

  const [selectedFilterField, setSelectedFilterField] = useState<IFilterField>();
  const [operators, setOperators] = useState<ReactElement[]>([]);
  const [selectedOperator, setSelectedOperator] = useState<string>();
  const [dateValue, setDateValue] = useState<Dayjs | null>();
  const [isCaseSensitive, setIsCaseSensitive] = useState(false);
  const [values, setValues] = useState<ReactElement[]>([]);
  const [value, setValue] = useState('');
  const { t } = useTranslation();

  useEffect(() => {
    if (dialogOpen) {
      setSelectedFilterField(existingFilter?.field);
      setSelectedOperator(existingFilter?.operator);
      if (existingFilter?.field.type === 'timestamp') {
        setDateValue(dayjs(existingFilter.value as number));
      } else {
        setValue(existingFilter?.value.toString() ?? '');
        setDateValue(null);
      }
      setIsCaseSensitive(existingFilter?.caseSensitive ?? false);
    }
  }, [dialogOpen]);

  useEffect(() => {
    if (selectedFilterField !== undefined) {
      let availableOperators: ReactElement[] = [];
      let availableValues: ReactElement[] = [];

      switch (selectedFilterField.type) {
        case 'boolean':
          availableOperators = [
            <MenuItem key="equal" value="equal">{t('equal')}</MenuItem>
          ];
          availableValues = [
            <MenuItem key="true" value="true">{t('true')}</MenuItem>,
            <MenuItem key="false" value="false">{t('false')}</MenuItem>
          ];
          break;
        case 'number':
          availableOperators = [
            <MenuItem key="equal" value="equal">{t('equal')}</MenuItem>,
            <MenuItem key="neq" value="neq">{t('notEqual')}</MenuItem>,
            <MenuItem key="greaterThan" value="greaterThan">{t('greaterThan')}</MenuItem>,
            <MenuItem key="greaterThanOrEqual" value="greaterThanOrEqual">{t('greaterThanOrEqual')}</MenuItem>,
            <MenuItem key="lessThan" value="lessThan">{t('lessThan')}</MenuItem>,
            <MenuItem key="lessThanOrEqual" value="lessThanOrEqual">{t('lessThanOrEqual')}</MenuItem>
          ];
          break;
        case 'string':
          if (selectedFilterField.isHexValue || selectedFilterField.isUUID) {
            availableOperators = [
              <MenuItem key="equal" value="equal">{t('equal')}</MenuItem>,
              <MenuItem key="neq" value="neq">{t('notEqual')}</MenuItem>
            ];
          } else {
            availableOperators = [
              <MenuItem key="equal" value="equal">{t('equal')}</MenuItem>,
              <MenuItem key="neq" value="neq">{t('notEqual')}</MenuItem>,
              <MenuItem key="contains" value="contains">{t('contains')}</MenuItem>,
              <MenuItem key="startsWith" value="startsWith">{t('startsWith')}</MenuItem>,
              <MenuItem key="endsWith" value="endsWith">{t('endsWith')}</MenuItem>,
              <MenuItem key="doesNotContain" value="doesNotContain">{t('doesNotContain')}</MenuItem>,
              <MenuItem key="doesNotStartWith" value="doesNotStartWith">{t('doesNotStartWith')}</MenuItem>,
              <MenuItem key="doesNotEndWith" value="doesNotEndWith">{t('doesNotEndWith')}</MenuItem>
            ];
          }
          break;
        case 'timestamp':
          availableOperators = [
            <MenuItem key="on" value="on">{t('on')}</MenuItem>,
            <MenuItem key="onOrAfter" value="onOrAfter">{t('onOrAfter')}</MenuItem>,
            <MenuItem key="onOrBefore" value="onOrBefore">{t('onOrBefore')}</MenuItem>,
            <MenuItem key="after" value="after">{t('after')}</MenuItem>,
            <MenuItem key="before" value="before">{t('before')}</MenuItem>
          ];
          break;
        case 'enum':
          availableOperators = [
            <MenuItem key="equal" value="equal">{t('equal')}</MenuItem>,
            <MenuItem key="neq" value="neq">{t('notEqual')}</MenuItem>,
          ];
          availableValues = selectedFilterField.enum!.map(entry =>
            <MenuItem key={entry} value={entry}>{t(entry)}</MenuItem>
          );
          break;
      }
      if (selectedOperator !== undefined && !availableOperators.some(operator => operator.key === selectedOperator)) {
        setSelectedOperator(undefined);
        setValue('');
        setDateValue(null);
      }
      setOperators(availableOperators);
      setValues(availableValues);
    }
  }, [selectedFilterField, selectedOperator, t]);

  useEffect(() => {
    if ((selectedFilterField?.isUUID || selectedFilterField?.isHexValue) && selectedOperator === 'equal') {
      setIsCaseSensitive(true);
    }
  }, [selectedFilterField, selectedOperator])

  const handleSubmit = () => {
    let newValue: string | number | boolean;
    switch (selectedFilterField?.type) {
      case 'boolean':
        newValue = value === 'true';
        break;
      case 'timestamp':
        newValue = dateValue!.toDate().getTime();
        break;
      case 'number':
        newValue = Number(value);
        break;
      case 'string':
        newValue = value;
        break;
      case 'enum':
        newValue = value;
        break;
    }
    if (selectedFilterField !== undefined && selectedOperator !== undefined) {
      if (existingFilter !== undefined) {
        existingFilter.field = selectedFilterField;
        existingFilter.operator = selectedOperator;
        existingFilter.value = newValue!;
        existingFilter.caseSensitive = selectedFilterField?.type === 'string' ? isCaseSensitive : undefined;
        updateFilters();
      }
      else {
        addFilter({
          field: selectedFilterField,
          operator: selectedOperator,
          value: newValue!,
          caseSensitive: selectedFilterField?.type === 'string' ? isCaseSensitive : undefined
        });
      }
      setDialogOpen(false);
    }
  };

  let valueHelperText: string | undefined = undefined;
  if (selectedFilterField?.type === 'number' && isNaN(Number(value))) {
    valueHelperText = t('mustBeANumber')
  } else if (selectedOperator !== undefined && ['equal', 'notEqual'].includes(selectedOperator)) {
    if (selectedFilterField?.isUUID && !isValidUUID(value)) {
      valueHelperText = t('mustBeAValidUUID')
    } else if (selectedFilterField?.isHexValue && !isValidHex(value)) {
      valueHelperText = t('mustBeAValidHex')
    }
  }

  const canSubmit = selectedFilterField !== undefined
    && selectedOperator !== undefined
    && ((selectedFilterField.type === 'timestamp' && dateValue !== null) || value.length > 0)
    && (selectedFilterField.type !== 'number' || !isNaN(Number(value)))
    && valueHelperText === undefined;

  return (
    <Dialog
      open={dialogOpen}
      onClose={() => setDialogOpen(false)}
      fullWidth
      maxWidth="xs"
    >
      <form onSubmit={(event) => {
        event.preventDefault();
        handleSubmit();
      }}>
        <DialogTitle sx={{ textAlign: 'center' }}>
          {t(existingFilter === undefined ? 'addFilter' : 'updateFilter')}
        </DialogTitle>
        <DialogContent>
          <Box sx={{ marginTop: '5px' }}>
            <Grid2 container spacing={2}>
              <Grid2 size={{ xs: 12 }}>
                <TextField
                  label={t('field')}
                  autoComplete="off"
                  fullWidth
                  value={selectedFilterField?.name ?? ''}
                  onChange={event => {
                    setSelectedFilterField(filterFields.find(filterField => filterField.name === event.target.value));
                    setSelectedOperator(undefined);
                    setValue('');
                    setDateValue(null);
                  }}
                  select
                >
                  {filterFields.map(filterField =>
                    <MenuItem sx={{ color: theme => filterField.isCustom ? theme.palette.primary.main : undefined }} key={filterField.name} value={filterField.name}>{filterField.label}</MenuItem>
                  )}
                </TextField>
              </Grid2>
              <Grid2 size={{ xs: 12 }} textAlign="center">
                <TextField
                  sx={{ textAlign: 'left' }}
                  label={t('operator')}
                  autoComplete="off"
                  fullWidth
                  value={selectedOperator ?? ''}
                  onChange={event => setSelectedOperator(event.target.value)}
                  select
                  disabled={selectedFilterField === undefined}
                >
                  {operators}
                </TextField>
              </Grid2>
              <Grid2 size={{ xs: 12 }}>
                {selectedFilterField?.type !== 'timestamp' &&
                  <TextField
                    error={value.length > 0 && valueHelperText !== undefined}
                    helperText={valueHelperText}
                    label={t('value')}
                    autoComplete="off"
                    fullWidth
                    disabled={selectedFilterField === undefined}
                    value={value}
                    onChange={event => setValue(event.target.value)}
                    select={selectedFilterField?.type === 'enum' || selectedFilterField?.type === 'boolean'}
                  >
                    {values}
                  </TextField>}
                {selectedFilterField?.type === 'timestamp' &&
                  <LocalizationProvider dateAdapter={AdapterDayjs}>
                    <DateTimePicker
                      sx={{ width: '100%' }}
                      format="MM/DD/YYYY hh:mm:ss A"
                      value={dateValue}
                      onChange={date => setDateValue(date)}
                      slotProps={{
                        textField: {
                          InputLabelProps: {
                            shrink: true
                          }
                        }
                      }}
                      label={t('timestamp')} />
                  </LocalizationProvider>}
                <Box sx={{ textAlign: 'center' }}>
                  <FormControlLabel
                    disabled={selectedFilterField === undefined || selectedFilterField.type !== 'string'
                      || ((selectedFilterField.isUUID || selectedFilterField.isHexValue) && selectedOperator === 'equal')
                    }
                    control={<Checkbox checked={isCaseSensitive} onChange={event => setIsCaseSensitive(event.target.checked)} />}
                    label={t('caseSensitive')} />
                </Box>
              </Grid2>
            </Grid2>
          </Box>
        </DialogContent>
        <DialogActions sx={{ justifyContent: 'center', paddingBottom: '20px' }}>
          <Button
            sx={{ minWidth: '100px' }}
            size="large"
            variant="contained"
            disableElevation
            disabled={!canSubmit}
            type="submit">
            {t(existingFilter === undefined ? 'add' : 'update')}
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
