import {
  coerceValue,
  compareValues,
  getFieldValue,
  hexVariants,
  likeToRegExp,
} from './compareValues.js';
import type { FieldMap, Op, OpMultiVal, OpSingleVal, Statements } from './types.js';

const evalSingleOp = (
  item: Record<string, unknown>,
  op: OpSingleVal,
  fieldMap: FieldMap,
  compare: 'eq' | 'lt' | 'lte' | 'gt' | 'gte' | 'like'
): boolean => {
  const field = op.field ?? '';
  const fieldType = fieldMap[field] ?? 'string';
  const itemValue = getFieldValue(item, field);
  const queryValue = coerceValue(op.value, fieldType);

  let matches = false;
  switch (compare) {
    case 'eq':
      matches = compareValues(itemValue, queryValue, fieldType, op.caseInsensitive) === 0;
      break;
    case 'lt':
      matches = compareValues(itemValue, queryValue, fieldType, op.caseInsensitive) < 0;
      break;
    case 'lte':
      matches = compareValues(itemValue, queryValue, fieldType, op.caseInsensitive) <= 0;
      break;
    case 'gt':
      matches = compareValues(itemValue, queryValue, fieldType, op.caseInsensitive) > 0;
      break;
    case 'gte':
      matches = compareValues(itemValue, queryValue, fieldType, op.caseInsensitive) >= 0;
      break;
    case 'like':
      matches = likeToRegExp(String(queryValue), op.caseInsensitive ?? false).test(
        String(itemValue ?? '')
      );
      break;
  }

  return op.not ? !matches : matches;
};

const evalMultiOp = (
  item: Record<string, unknown>,
  op: OpMultiVal,
  fieldMap: FieldMap,
  mode: 'in' | 'nin'
): boolean => {
  const field = op.field ?? '';
  const fieldType = fieldMap[field] ?? 'string';
  const itemValue = getFieldValue(item, field);
  const values = op.values ?? [];

  let matches: boolean;
  if (fieldType === 'hex') {
    const itemVariants = new Set(hexVariants(itemValue));
    matches = values.some((value) =>
      hexVariants(value).some((variant) => itemVariants.has(variant))
    );
  } else {
    matches = values.some(
      (value) =>
        compareValues(itemValue, coerceValue(value, fieldType), fieldType, op.caseInsensitive) ===
        0
    );
  }

  const result = mode === 'nin' ? !matches : matches;
  return op.not ? !result : result;
};

const evalNullOp = (item: Record<string, unknown>, op: Op, fieldMap: FieldMap): boolean => {
  const field = op.field ?? '';
  const value = getFieldValue(item, field);
  const isNull = value === null || value === undefined;
  return op.not ? !isNull : isNull;
};

const evalOps = (item: Record<string, unknown>, statements: Statements, fieldMap: FieldMap): boolean => {
  const checks: boolean[] = [];

  const pushOps = (
    ops: OpSingleVal[] | undefined,
    compare: 'eq' | 'lt' | 'lte' | 'gt' | 'gte' | 'like'
  ) => {
    for (const op of ops ?? []) {
      checks.push(evalSingleOp(item, op, fieldMap, compare));
    }
  };

  pushOps(statements.equal, 'eq');
  pushOps(statements.eq, 'eq');
  for (const op of statements.neq ?? []) {
    checks.push(!evalSingleOp(item, op, fieldMap, 'eq'));
  }
  pushOps(statements.like, 'like');
  pushOps(statements.lessThan, 'lt');
  pushOps(statements.lt, 'lt');
  pushOps(statements.lessThanOrEqual, 'lte');
  pushOps(statements.lte, 'lte');
  pushOps(statements.greaterThan, 'gt');
  pushOps(statements.gt, 'gt');
  pushOps(statements.greaterThanOrEqual, 'gte');
  pushOps(statements.gte, 'gte');

  for (const op of statements.in ?? []) {
    checks.push(evalMultiOp(item, op, fieldMap, 'in'));
  }
  for (const op of statements.nin ?? []) {
    checks.push(evalMultiOp(item, op, fieldMap, 'nin'));
  }
  for (const op of statements.null ?? []) {
    checks.push(evalNullOp(item, op, fieldMap));
  }

  if (checks.length === 0) {
    return true;
  }

  return checks.every(Boolean);
};

export const evalStatements = (
  item: Record<string, unknown>,
  statements: Statements,
  fieldMap: FieldMap
): boolean => {
  if (statements.or !== undefined && statements.or.length > 0) {
    const orMatches = statements.or.some((branch) => evalStatements(item, branch, fieldMap));
    const andMatches = evalOps(item, statements, fieldMap);
    return orMatches && andMatches;
  }

  return evalOps(item, statements, fieldMap);
};
