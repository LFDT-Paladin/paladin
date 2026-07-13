import type { FieldType } from './types.js';

const HEX_PREFIX = /^0x/i;

export const normalizeHex = (value: unknown): string => {
  const str = String(value).toLowerCase();
  return str.startsWith('0x') ? str : `0x${str}`;
};

export const hexVariants = (value: unknown): string[] => {
  const normalized = normalizeHex(value);
  const stripped = normalized.slice(2);
  return [normalized, stripped];
};

export const coerceValue = (value: unknown, type: FieldType): unknown => {
  if (value === null || value === undefined) {
    return value;
  }

  switch (type) {
    case 'int':
      return typeof value === 'number' ? value : Number(value);
    case 'float':
      return typeof value === 'number' ? value : Number(value);
    case 'bool':
      return typeof value === 'boolean' ? value : value === 'true' || value === true;
    case 'hex':
      return normalizeHex(value);
    case 'timestamp':
      if (typeof value === 'number') {
        return value;
      }
      const asNumber = Number(value);
      if (!Number.isNaN(asNumber)) {
        return asNumber;
      }
      const asDate = Date.parse(String(value));
      return Number.isNaN(asDate) ? String(value) : asDate;
    default:
      return String(value);
  }
};

export const compareValues = (
  left: unknown,
  right: unknown,
  type: FieldType,
  caseInsensitive = false
): number => {
  if (left === null || left === undefined) {
    return right === null || right === undefined ? 0 : -1;
  }
  if (right === null || right === undefined) {
    return 1;
  }

  if (type === 'hex') {
    const a = normalizeHex(left);
    const b = normalizeHex(right);
    return a.localeCompare(b);
  }

  if (type === 'bool') {
    return Number(Boolean(left)) - Number(Boolean(right));
  }

  if (type === 'int' || type === 'float' || type === 'timestamp') {
    const a = coerceValue(left, type);
    const b = coerceValue(right, type);
    if (typeof a === 'number' && typeof b === 'number') {
      return a - b;
    }
    return String(a).localeCompare(String(b));
  }

  const a = caseInsensitive ? String(left).toLowerCase() : String(left);
  const b = caseInsensitive ? String(right).toLowerCase() : String(right);
  return a.localeCompare(b);
};

export const likeToRegExp = (pattern: string, caseInsensitive: boolean): RegExp => {
  const escaped = pattern.replace(/[.+^${}()|[\]\\]/g, '\\$&');
  const regexSource = `^${escaped.replace(/%/g, '.*').replace(/_/g, '.')}$`;
  return new RegExp(regexSource, caseInsensitive ? 'i' : '');
};

export const getFieldValue = (item: Record<string, unknown>, field: string): unknown => {
  const key = field.startsWith('.') ? field.slice(1) : field;
  if (Object.prototype.hasOwnProperty.call(item, key)) {
    return item[key];
  }
  const properties = item.properties;
  if (
    properties !== null &&
    typeof properties === 'object' &&
    !Array.isArray(properties) &&
    Object.prototype.hasOwnProperty.call(properties, key)
  ) {
    return (properties as Record<string, unknown>)[key];
  }
  const data = item.data;
  if (
    data !== null &&
    typeof data === 'object' &&
    !Array.isArray(data) &&
    Object.prototype.hasOwnProperty.call(data, key)
  ) {
    return (data as Record<string, unknown>)[key];
  }
  return undefined;
};
