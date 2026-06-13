// Copyright © 2025 Kaleido, Inc.
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

import { Box, Button, Chip } from "@mui/material";
import { Dispatch, SetStateAction, useState } from "react";
import { useTranslation } from "react-i18next";
import { IFilter, IFilterField } from "../interfaces";
import AddIcon from '@mui/icons-material/Add';
import ClearAllIcon from '@mui/icons-material/ClearAll';
import { FilterDialog } from "../dialogs/Filter";

type Props = {
  filterFields: IFilterField[]
  filters: IFilter[]
  setFilters: Dispatch<SetStateAction<IFilter[]>>
}

export const Filters: React.FC<Props> = ({
  filterFields,
  filters,
  setFilters
}) => {

  const [addFilterDialogOpen, setAddFilterDialogOpen] = useState(false);
  const [selectedFilter, setSelectedFilter] = useState<IFilter>();
  const { t } = useTranslation();

  const getOperatorLabel = (operator: string) => {
    switch (operator) {
      case 'equal': return '= ';
      case 'neq': return '!= ';
      case 'greaterThan': return '> ';
      case 'greaterThanOrEqual': return '>= ';
      case 'lessThan': return '< ';
      case 'lessThanOrEqual': return '<= ';
      case 'contains': return '= @';
      case 'startsWith': return '= ^';
      case 'endsWith': return '= $';
      case 'doesNotContain': return '= !@';
      case 'doesNotStartWith': return '= !^';
      case 'doesNotEndWith': return '= !$';
      case 'on': return '= ';
      case 'after': return '> ';
      case 'onOrAfter': return '>= ';
      case 'before': return '< ';
      case 'onOrBefore': return '<= ';
    }
  };

  const getFilterId = (filter: IFilter) => `${filter.field.name}-${filter.operator}-${filter.value}${filter.caseSensitive}`;

  const generateFilterLabelValue = (filter: IFilter) => {
    if (filter.field.type === 'enum') {
      return t(filter.value as string);
    } else if(filter.field.type === 'timestamp') {
      return new Date(filter.value as number).toLocaleString();
    }
    return filter.value;
  }

  const generateFilterLabel = (filter: IFilter) => {
    return `${filter.field.label} ${getOperatorLabel(filter.operator)}${generateFilterLabelValue(filter)}`
  };

  return (
    <>
      <Box sx={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'right',
        gap: '10px',
        flexWrap: 'wrap',
        backgroundColor: theme => theme.palette.background.paper,
        padding: '6px',
        borderRadius: '20px'
      }}>

        {filters.map(filter =>
          <Chip
            key={getFilterId(filter)}
            label={generateFilterLabel(filter)}
            onClick={() => {setSelectedFilter(filter); setAddFilterDialogOpen(true)}}
            onDelete={() => {
              const id = getFilterId(filter);
              setFilters(filters.filter(currentFilter => getFilterId(currentFilter) !== id));
            }}
          />
        )}

        {filters.length > 0 &&
          <Button
            size="small"
            variant="outlined"
            sx={{ borderRadius: '20px', minWidth: '120px' }}
            onClick={() => setFilters([])}
            startIcon={<ClearAllIcon />}
          >
            {t('clearFilters')}
          </Button>
        }

        <Button
          size="small"
          variant="outlined"
          color="secondary"
          sx={{ borderRadius: '20px', minWidth: '120px' }}
          onClick={() => { setSelectedFilter(undefined); setAddFilterDialogOpen(true) }}
          startIcon={<AddIcon />}
        >
          {t('addFilter')}
        </Button>

      </Box>

      {/* <AddFilterDialog
        filterFields={filterFields}
        addFilter={filter => setFilters([...filters, filter])}
        dialogOpen={addFilterDialogOpen}
        setDialogOpen={setAddFilterDialogOpen}
      /> */}

      <FilterDialog
        existingFilter={selectedFilter}
        filterFields={filterFields}
        addFilter={filter => setFilters([...filters, filter])}
        updateFilters={() => setFilters([...filters])}
        dialogOpen={addFilterDialogOpen}
        setDialogOpen={setAddFilterDialogOpen}
      />

    </>
  );

}