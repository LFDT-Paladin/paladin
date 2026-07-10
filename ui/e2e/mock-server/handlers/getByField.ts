import { compareValues, hexVariants } from '../query/compareValues.js';
import { getFieldMap, loadCollection } from '../store/dataStore.js';
import type { MethodConfig } from './registry.js';

const valuesMatch = (
  itemValue: unknown,
  paramValue: unknown,
  field: string,
  collection: string
): boolean => {
  const fieldType = getFieldMap(collection)[field] ?? 'string';

  if (fieldType === 'hex') {
    const paramVariants = new Set(hexVariants(paramValue));
    return hexVariants(itemValue).some((variant) => paramVariants.has(variant));
  }

  return compareValues(itemValue, paramValue, fieldType) === 0;
};

export const handleGetByField = (
  config: MethodConfig,
  params: unknown[]
): unknown => {
  const collection = config.collection;
  const field = config.field;
  if (collection === undefined || field === undefined) {
    return null;
  }

  const paramValue = params[config.paramIndex ?? 0];
  const items = loadCollection(collection);
  const match = items.find((item) => valuesMatch(item[field], paramValue, field, collection));

  if (match === undefined) {
    return null;
  }

  if (config.returnField !== undefined) {
    return match[config.returnField] ?? null;
  }

  return match;
};
