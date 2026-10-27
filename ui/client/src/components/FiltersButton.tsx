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

import { Button } from "@mui/material"
import { Dispatch, SetStateAction } from "react"
import { useTranslation } from "react-i18next"
import FilterAltIcon from '@mui/icons-material/FilterAlt';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import ExpandLessIcon from '@mui/icons-material/ExpandLess';

type Props = {
  filtersVisible: boolean
  setFiltersVisible: Dispatch<SetStateAction<boolean>>
}

export const FiltersButton: React.FC<Props> = ({
  filtersVisible,
  setFiltersVisible
}) => {

  const { t } = useTranslation();
  return (
    <Button
      sx={{ borderRadius: '20px', minWidth: '120px' }}
      size="small"
      color="secondary"
      variant="outlined"
      startIcon={<FilterAltIcon />}
      endIcon={filtersVisible ? <ExpandLessIcon /> : <ExpandMoreIcon />}
      onClick={() => setFiltersVisible(!filtersVisible)}
    >
      {t('filters')}
    </Button>
  );

}
