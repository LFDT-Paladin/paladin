import { loadStatic } from '../store/dataStore.js';
import { handleDerivedQuery } from './derivedQuery.js';
import { handleGetByField } from './getByField.js';
import { handleQueryList } from './queryList.js';
import { getMethodConfig } from './registry.js';
import type { MethodConfig } from './registry.js';

export const handleRpcMethod = async (
  method: string,
  params: unknown[] = []
): Promise<unknown> => {
  const config = getMethodConfig(method);
  if (config === undefined) {
    console.warn(`[mock-rpc] unhandled method: ${method}, returning []`);
    return [];
  }

  return dispatchMethod(config, params);
};

const dispatchMethod = (config: MethodConfig, params: unknown[]): unknown => {
  switch (config.type) {
    case 'query':
      return handleQueryList(config, params);
    case 'derivedQuery':
      return handleDerivedQuery(config, params);
    case 'getByField':
      return handleGetByField(config, params);
    case 'static':
      return config.file !== undefined ? loadStatic(config.file) : config.returnValue ?? null;
    case 'empty':
      return config.returnValue ?? [];
    default:
      return [];
  }
};
