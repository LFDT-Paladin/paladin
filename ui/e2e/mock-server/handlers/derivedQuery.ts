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

  let filtered = loadCollection(collection);

  if (
    preFilter.hasReceiptIn !== undefined &&
    preFilter.joinField !== undefined &&
    preFilter.sourceField !== undefined
  ) {
    const relatedItems = loadCollection(preFilter.hasReceiptIn);
    const relatedValues = new Set(
      relatedItems.map((item) => String(item[preFilter.joinField!] ?? ''))
    );
    filtered = filtered.filter((item) =>
      relatedValues.has(String(item[preFilter.sourceField!] ?? ''))
    );
  }

  if (preFilter.fieldMissing !== undefined) {
    const field = preFilter.fieldMissing;
    filtered = filtered.filter(
      (item) => item[field] === undefined || item[field] === null
    );
  }

  if (preFilter.equalField !== undefined) {
    const expected = params[preFilter.equalParamIndex ?? 0];
    const field = preFilter.equalField;
    filtered = filtered.filter((item) => item[field] === expected);
  }

  if (preFilter.activeParamIndex !== undefined) {
    const activeFilter = String(params[preFilter.activeParamIndex] ?? 'any');
    if (activeFilter === 'active') {
      filtered = filtered.filter((item) => item.active !== false);
    } else if (activeFilter === 'inactive') {
      filtered = filtered.filter((item) => item.active === false);
    }
    // any / all → no active filter
  }

  const query = (params[config.queryParamIndex ?? 0] ?? {}) as QueryJSON;
  return applyQuery(filtered, query, getFieldMap(collection));
};
