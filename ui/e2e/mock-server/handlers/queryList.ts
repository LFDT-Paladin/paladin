import { applyQuery } from '../query/applyQuery.js';
import type { QueryJSON } from '../query/types.js';
import { getFieldMap, loadCollection } from '../store/dataStore.js';
import type { MethodConfig } from './registry.js';

export const handleQueryList = (
  config: MethodConfig,
  params: unknown[]
): Record<string, unknown>[] => {
  const collection = config.collection;
  if (collection === undefined) {
    return [];
  }

  const query = (params[config.queryParamIndex ?? 0] ?? {}) as QueryJSON;
  const items = loadCollection(collection);
  return applyQuery(items, query, getFieldMap(collection));
};
