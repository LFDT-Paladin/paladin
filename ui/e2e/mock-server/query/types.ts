export type FieldType = 'string' | 'int' | 'float' | 'bool' | 'hex' | 'timestamp';

export type FieldMap = Record<string, FieldType>;

export interface Op {
  not?: boolean;
  caseInsensitive?: boolean;
  field?: string;
}

export interface OpSingleVal extends Op {
  value?: unknown;
}

export interface OpMultiVal extends Op {
  values?: unknown[];
}

export interface Statements {
  or?: Statements[];
  equal?: OpSingleVal[];
  eq?: OpSingleVal[];
  neq?: OpSingleVal[];
  like?: OpSingleVal[];
  lessThan?: OpSingleVal[];
  lt?: OpSingleVal[];
  lessThanOrEqual?: OpSingleVal[];
  lte?: OpSingleVal[];
  greaterThan?: OpSingleVal[];
  gt?: OpSingleVal[];
  greaterThanOrEqual?: OpSingleVal[];
  gte?: OpSingleVal[];
  in?: OpMultiVal[];
  nin?: OpMultiVal[];
  null?: Op[];
}

export interface QueryJSON extends Statements {
  limit?: number;
  sort?: string[];
}

export interface SortField {
  field: string;
  descending: boolean;
}
