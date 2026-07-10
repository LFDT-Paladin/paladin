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
