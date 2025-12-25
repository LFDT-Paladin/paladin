import { Box, Button, Dialog, DialogContent, IconButton, Tooltip, Typography } from "@mui/material";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { JSONBox } from "../components/JSONBox";
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import { SingleValue } from "../components/SingleValue";
import { queryPrivacyGroupByAddress } from "../queries/privacyGroups";
import { useQuery } from "@tanstack/react-query";

type Props = {
  title: string;
  hash: string;
  dialogOpen: boolean;
  setDialogOpen: (open: boolean) => void;
  config: any;
  domain?: string;
}



export const SmartContractDetails: React.FC<Props> = ({ title, hash, dialogOpen, setDialogOpen, config, domain }) => {

  const { t } = useTranslation();

  const NotoDetails: React.FC<Pick<Props, 'config'>> = ({ config }) => {
    return (
     <div style={{ display: 'flex', flexDirection: 'column', gap: '5px' }}>
       <SingleValue label={t('name')} value={config?.name} />
       <SingleValue label={t('notary')} value={config?.notaryLookup} />
       <SingleValue label={t('notaryMode')} value={config?.notaryMode} />
       <SingleValue label={t('symbol')} value={config?.symbol} />
       <SingleValue label={t('decimals')} value={config?.decimals} />
       <SingleValue label={t('options')} value={config?.options} json={true} />
     </div>
    )
   }
   
   const ZetoDetails: React.FC<Pick<Props, 'config'>> = ({ config }) => {
     return (
       <div style={{ display: 'flex', flexDirection: 'column', gap: '5px' }}>
         <SingleValue label={t('name')} value={config?.tokenName} />
         <SingleValue label={t('circuits')} value={config?.circuits} json={true} />
       </div>
     )
   }
   
   const PenteDetails: React.FC<Pick<Props, 'config' | 'hash'>> = ({ config, hash }) => {

    const privacyGroupByAddress = useQuery({
      queryKey: ['privacyGroupByAddress', hash],
      queryFn: () => queryPrivacyGroupByAddress(hash),
      enabled: !!hash,
    });

     return (
       <div style={{ display: 'flex', flexDirection: 'column', gap: '5px' }}>
         <SingleValue label={t('evmVersion')} value={config?.evmVersion} />
         <SingleValue label={t('id')} value={privacyGroupByAddress.data?.id} />
         <SingleValue label={t('name')} value={privacyGroupByAddress.data?.name} />
         <SingleValue label={t('members')} value={privacyGroupByAddress.data?.members.join(', ')} />
         <SingleValue label={t('configuration')} value={privacyGroupByAddress.data?.configuration} json={true} />
         <SingleValue label={t('properties')} value={privacyGroupByAddress.data?.properties} json={true} />
       </div>
     )
   }

  return (
      <Dialog open={dialogOpen} onClose={() => setDialogOpen(false)} maxWidth="lg">
        <DialogContent sx={{ display: 'flex', flexDirection: 'column', gap: '5px' }}>
          <SingleValue label={title} value={hash} />
          {domain === 'noto' && <NotoDetails config={config} />}
          {domain === 'zeto' && <ZetoDetails config={config} />}
          {domain === 'pente' && <PenteDetails config={config} hash={hash} />}
        </DialogContent>
      </Dialog>
  );
};