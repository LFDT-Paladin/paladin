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
