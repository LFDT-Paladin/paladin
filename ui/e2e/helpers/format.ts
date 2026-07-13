export const getShortHash = (hash: string): string => {
  if (hash.length < 16) {
    return hash;
  }
  return `${hash.substring(0, 6)}...${hash.substring(hash.length - 4)}`;
};

export const getShortId = (value: string): string => {
  if (value.length < 16) {
    return value;
  }
  return `${value.substring(0, 4)}...${value.substring(value.length - 4)}`;
};

export {
  formatTxHash,
  formatFromAddress,
  formatToAddress,
  formatReceiptId,
  formatEventHash,
  TRANSACTION_COUNT,
} from '../mock-server/fixtures/transaction-data.js';

export {
  formatSubmissionId,
  formatSubmissionFrom,
  formatSubmissionTo,
  SUBMISSION_COUNT,
  submissionStatusForIndex,
} from '../mock-server/fixtures/submission-data.js';

export {
  formatKeyEthAddress,
  formatKeyHandle,
  KEY_COUNT,
  ROOT_KEY_COUNT,
  ORG1_KEY_COUNT,
  ORG2_KEY_COUNT,
  SUBORG1_KEY_COUNT,
} from '../mock-server/fixtures/key-data.js';

export {
  formatContractAddress,
  formatNotoRegistryAddress,
  formatPenteRegistryAddress,
  formatZetoRegistryAddress,
  DOMAIN_NAMES,
  NOTO_CONTRACT_COUNT,
  ZETO_CONTRACT_COUNT,
  PENTE_CONTRACT_COUNT,
  SMART_CONTRACT_COUNT,
} from '../mock-server/fixtures/domain-data.js';

export {
  formatRegistryEntryId,
  formatRegistryOwner,
  REGISTRY_NAMES,
  REGISTRY_ENTRY_COUNT,
  REGISTRY_INACTIVE_COUNT,
} from '../mock-server/fixtures/registry-data.js';

export {
  formatPrivacyGroupId,
  formatPrivacyGroupAddress,
  formatMessageId,
  formatGenesisTxId,
  PRIVACY_GROUP_COUNT,
  PRIVACY_GROUP_MESSAGE_COUNT,
  PRIVACY_GROUP_LISTENER_COUNT,
  PRIVACY_GROUP_LISTENER_STARTED_COUNT,
} from '../mock-server/fixtures/privacy-group-data.js';

export {
  formatSchemaId,
  formatStateId,
  formatStateContractAddress,
  formatStateOwner,
  SCHEMA_IDS,
  SCHEMA_COUNT,
  STATE_COUNT,
  NOTO_COIN_STATE_COUNT,
  NOTO_LOCKED_COIN_STATE_COUNT,
  ZETO_COIN_STATE_COUNT,
  PENTE_ACCOUNT_STATE_COUNT,
} from '../mock-server/fixtures/state-data.js';

export {
  formatPeerName,
  formatReliableMessageId,
  TRANSPORT_PEER_COUNT,
  TRANSPORT_MESSAGE_COUNT,
  MESSAGE_TYPES,
} from '../mock-server/fixtures/transport-data.js';
