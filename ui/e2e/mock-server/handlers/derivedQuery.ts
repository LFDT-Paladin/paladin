import { applyQuery } from '../query/applyQuery.js';
import type { QueryJSON } from '../query/types.js';
import { getFieldMap, loadCollection } from '../store/dataStore.js';
import type { MethodConfig } from './registry.js';

export const handleDerivedQuery = (
  config: MethodConfig,
  params: unknown[]
): Record<string, unknown>[] => {
  const collection = config.collection;
  const preFilter = config.preFilter;
  if (collection === undefined || preFilter === undefined) {
    return [];
  }

  const sourceItems = loadCollection(collection);
  const relatedItems = loadCollection(preFilter.hasReceiptIn);
  const relatedValues = new Set(
    relatedItems.map((item) => String(item[preFilter.joinField] ?? ''))
  );

  const filtered = sourceItems.filter((item) =>
    relatedValues.has(String(item[preFilter.sourceField] ?? ''))
  );

  const query = (params[config.queryParamIndex ?? 0] ?? {}) as QueryJSON;
  return applyQuery(filtered, query, getFieldMap(collection));
};
