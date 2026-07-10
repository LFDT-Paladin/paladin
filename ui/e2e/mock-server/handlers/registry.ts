import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

export interface PreFilterConfig {
  hasReceiptIn: string;
  joinField: string;
  sourceField: string;
}

export interface MethodConfig {
  type: 'query' | 'derivedQuery' | 'getByField' | 'static' | 'empty';
  collection?: string;
  queryParamIndex?: number;
  field?: string;
  paramIndex?: number;
  returnField?: string;
  file?: string;
  returnValue?: unknown;
  preFilter?: PreFilterConfig;
}

const rootDir = dirname(fileURLToPath(import.meta.url));

let methodRegistry: Record<string, MethodConfig> | null = null;

export const getMethodRegistry = (): Record<string, MethodConfig> => {
  if (methodRegistry === null) {
    methodRegistry = JSON.parse(
      readFileSync(join(rootDir, '../methods.json'), 'utf-8')
    ) as Record<string, MethodConfig>;
  }
  return methodRegistry;
};

export const getMethodConfig = (method: string): MethodConfig | undefined =>
  getMethodRegistry()[method];
