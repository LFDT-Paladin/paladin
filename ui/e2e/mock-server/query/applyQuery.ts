import { compareValues, getFieldValue } from './compareValues.js';
import { evalStatements } from './evalStatements.js';
import type { FieldMap, QueryJSON, SortField } from './types.js';

export const parseSortFields = (sort: string[] | undefined): SortField[] =>
  (sort ?? []).map((entry) => {
    const trimmed = entry.trim();
    if (trimmed.startsWith('-')) {
      return { field: trimmed.slice(1), descending: true };
    }
    const parts = trimmed.split(/\s+/);
    if (parts.length > 1 && parts[1].toUpperCase() === 'DESC') {
      return { field: parts[0], descending: true };
    }
    return { field: parts[0], descending: false };
  });

export const applyQuery = <T extends Record<string, unknown>>(
  items: T[],
  query: QueryJSON,
  fieldMap: FieldMap
): T[] => {
  let results = items.filter((item) => evalStatements(item, query, fieldMap));

  const sortFields = parseSortFields(query.sort);
  if (sortFields.length > 0) {
    results = [...results].sort((a, b) => {
      for (const sortField of sortFields) {
        const fieldType = fieldMap[sortField.field] ?? 'string';
        const cmp = compareValues(
          getFieldValue(a, sortField.field),
          getFieldValue(b, sortField.field),
          fieldType
        );
        if (cmp !== 0) {
          return sortField.descending ? -cmp : cmp;
        }
      }
      return 0;
    });
  }

  if (query.limit !== undefined && query.limit >= 0) {
    results = results.slice(0, query.limit);
  }

  return results;
};
