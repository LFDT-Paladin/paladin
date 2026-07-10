import { loadCollection } from '../store/dataStore.js';
import type { MethodConfig } from './registry.js';

/**
 * Returns an array of a single field from every item in a collection.
 * Used for APIs like domain_listDomains that return name strings, not objects.
 */
export const handleListField = (config: MethodConfig): unknown => {
  const collection = config.collection;
  const field = config.field;
  if (collection === undefined || field === undefined) {
    return [];
  }

  return loadCollection(collection).map((item) => item[field] ?? null);
};
