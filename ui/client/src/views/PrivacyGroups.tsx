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

import { Alert, Box, Button, Fade, Grid2, IconButton, Paper, Table, TableBody, TableCell, TableContainer, TableHead, TablePagination, TableRow, TableSortLabel, Tooltip, Typography } from "@mui/material";
import { Dispatch, SetStateAction, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import SearchIcon from '@mui/icons-material/Search';
import { listPrivacyGroups } from "../queries/privacyGroups";
import { useQuery } from "@tanstack/react-query";
import { Timestamp } from "../components/Timestamp";
import { Hash } from "../components/Hash";
import { customNavigate } from "../utils";
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import { useNavigate } from "react-router-dom";
import { PrivacyGroupMembers } from "../components/PrivacyGroupMembers";
import { Captions, Tag } from "lucide-react";
import { PrivacyGroupLookupDialog } from "../dialogs/PrivacyGroupLookup";
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';

type Props = {
  sortAscending: boolean
  setSortAscending: Dispatch<SetStateAction<boolean>>
  refTimestamps: string[]
  setRefTimestamps: Dispatch<SetStateAction<string[]>>
  page: number
  setPage: Dispatch<SetStateAction<number>>
  rowsPerPage: number
  setRowsPerPage: Dispatch<SetStateAction<number>>
};

export const PrivacyGroups: React.FC<Props> = ({
  sortAscending,
  setSortAscending,
  refTimestamps,
  setRefTimestamps,
  page,
  setPage,
  rowsPerPage,
  setRowsPerPage,
}) => {

  const [lookupPrivacyGroupDialogOpen, setLookupPrivacyGroupDialogOpen] = useState(false);
  const navigate = useNavigate();
  const [count, setCount] = useState(-1);
  const { t } = useTranslation();

  const { data: privacyGroups, error } = useQuery({
    queryKey: ['privacyGroups', page, rowsPerPage, sortAscending],
    queryFn: () => listPrivacyGroups(rowsPerPage, sortAscending, refTimestamps[refTimestamps.length - 1]),
  });

  if (error) {
    return (
      <Alert sx={{ margin: '30px' }} severity="error" variant="filled">
        {error.message}
      </Alert>
    );
  }

  useEffect(() => {
    if (privacyGroups !== undefined && count === -1) {
      if (privacyGroups.length < rowsPerPage) {
        setCount(rowsPerPage * page + privacyGroups.length);
      }
    }
  }, [privacyGroups, rowsPerPage, page]);

  const handleChangePage = (
    _event: React.MouseEvent<HTMLButtonElement> | null,
    newPage: number
  ) => {
    if (newPage === 0) {
      setRefTimestamps([]);
    } else if (newPage > page) {
      if (privacyGroups !== undefined) {
        const refEntriesCopy = [...refTimestamps];
        refEntriesCopy.push(privacyGroups[privacyGroups.length - 1].created);
        setRefTimestamps(refEntriesCopy);
      }
    } else {
      const refEntriesCopy = [...refTimestamps];
      refEntriesCopy.pop();
      setRefTimestamps(refEntriesCopy);
    }
    setPage(newPage);
  };

  const handleChangeRowsPerPage = (
    event: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>
  ) => {
    const value = parseInt(event.target.value, 10);
    setRowsPerPage(value);
    setRefTimestamps([]);
    setPage(0);
  };

  return (
    <>
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
            <Grid2 container alignItems="center" spacing={2}>
              <Grid2 sx={{ display: { xs: 'none', sm: 'none', md: 'block' } }} size={{ md: 4 }} />
              <Grid2 size={{ xs: 12, md: 4 }}>
                <Typography align="center" variant="h5">
                  {t("privacyGroups")}
                </Typography>
              </Grid2>
              <Grid2 size={{ xs: 12, md: 4 }} container justifyContent="right">
                <Grid2>
                  <Button
                    sx={{ borderRadius: '20px', minWidth: '180px' }}
                    size="large"
                    variant="outlined"
                    startIcon={<SearchIcon />}
                    onClick={() => setLookupPrivacyGroupDialogOpen(true)}
                  >
                    {t('lookup')}
                  </Button>
                </Grid2>
              </Grid2>
            </Grid2>
          </Box>
          <Box sx={{
            display: 'flex',
            flexDirection: 'column',
            gap: '20px'
          }}>
            {privacyGroups !== undefined && privacyGroups.length > 0 &&
              <TableContainer
                component={Paper}
              >
                <Table stickyHeader>
                  <TableHead>
                    <TableRow>
                      <TableCell
                        width={1}
                        sx={{
                          backgroundColor: (theme) => theme.palette.background.paper,
                        }}>
                        <TableSortLabel
                          active={true}
                          direction={sortAscending ? 'asc' : 'desc'}
                          onClick={() => {
                            setSortAscending(!sortAscending);
                            setRefTimestamps([]);
                            setPage(0);
                          }}
                        >
                          {t('created')}
                        </TableSortLabel>
                      </TableCell>
                      <TableCell
                        width={1}
                        sx={{
                          backgroundColor: (theme) => theme.palette.background.paper,
                          whiteSpace: 'nowrap'
                        }}
                      >
                        {t('id')}
                      </TableCell>
                      <TableCell
                        width={1}
                        sx={{
                          backgroundColor: (theme) => theme.palette.background.paper,
                          whiteSpace: 'nowrap'
                        }}
                      >
                        {t('name')}
                      </TableCell>
                      <TableCell
                        width={1}
                        sx={{
                          backgroundColor: (theme) => theme.palette.background.paper,
                          whiteSpace: 'nowrap'
                        }}
                      >
                        {t('domain')}
                      </TableCell>
                      <TableCell
                        width={1}
                        sx={{
                          backgroundColor: (theme) => theme.palette.background.paper,
                          whiteSpace: 'nowrap'
                        }}
                      >
                        {t('contractAddress')}
                      </TableCell>
                      <TableCell
                        sx={{
                          backgroundColor: (theme) => theme.palette.background.paper,
                          whiteSpace: 'nowrap',
                          width: '100%'
                        }}
                      >
                        {t('members')}
                      </TableCell>
                      <TableCell
                        width={1}
                        sx={{
                          backgroundColor: (theme) => theme.palette.background.paper,
                          whiteSpace: 'nowrap'
                        }}
                      >
                      </TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {privacyGroups?.map(privacyGroup =>
                      <TableRow key={privacyGroup.id}>
                        <TableCell >
                          <Timestamp timestamp={privacyGroup.created} />
                        </TableCell>
                        <TableCell>
                          <Hash Icon={<Tag size="18px" />} title={t('id')} hash={privacyGroup.id} />
                        </TableCell>
                        <TableCell>
                          {privacyGroup.name.length > 0 ? privacyGroup.name : '--'}
                        </TableCell>
                        <TableCell>
                          {t(privacyGroup.domain)}
                        </TableCell>
                        <TableCell>
                          <Hash Icon={<Captions size="18px" />} title={t('address')} hash={privacyGroup.contractAddress} />
                        </TableCell>
                        <TableCell sx={{ maxWidth: 0, overflow: 'hidden', p: 0 }}>
                          <PrivacyGroupMembers members={privacyGroup.members} />
                        </TableCell>
                        <TableCell align="right" sx={{ padding: '8px' }}>
                          <Tooltip title={t('open')} arrow>
                            <IconButton
                              onClick={mouseEvent => customNavigate(`/ui/privacy-groups/${privacyGroup.id}`, mouseEvent, navigate)}>
                              <OpenInNewIcon color="secondary" fontSize="medium" />
                            </IconButton>
                          </Tooltip>
                        </TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                </Table>
                <TablePagination
                  slotProps={{
                    actions: {
                      lastButton: {
                        disabled: true
                      }
                    }
                  }}
                  component="div"
                  showFirstButton
                  showLastButton
                  count={count}
                  page={page}
                  onPageChange={handleChangePage}
                  rowsPerPage={rowsPerPage}
                  onRowsPerPageChange={handleChangeRowsPerPage}
                />
              </TableContainer>}
            {privacyGroups !== undefined && privacyGroups.length === 0 &&
              <Box sx={{ marginTop: '60px', textAlign: 'center', color: theme => theme.palette.text.secondary }}>
                <InfoOutlinedIcon sx={{ fontSize: '50px' }} />
                <Typography>{t('privacyGroupsEmptyState')}</Typography>
              </Box>
            }
          </Box>
        </Box>
      </Fade>
      <PrivacyGroupLookupDialog
        dialogOpen={lookupPrivacyGroupDialogOpen}
        setDialogOpen={setLookupPrivacyGroupDialogOpen}
      />
    </>
  );

}