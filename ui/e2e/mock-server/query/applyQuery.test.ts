import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { applyQuery } from './applyQuery.js';
import type { FieldMap, QueryJSON } from './types.js';

const txFieldMap: FieldMap = {
  hash: 'hex',
  blockNumber: 'int',
  transactionIndex: 'int',
  from: 'hex',
  to: 'hex',
  nonce: 'int',
  result: 'string',
};

const sampleTransactions = [
  { hash: '0x01', blockNumber: 100, transactionIndex: 0, result: 'success' },
  { hash: '0x02', blockNumber: 99, transactionIndex: 0, result: 'success' },
  { hash: '0x03', blockNumber: 90, transactionIndex: 1, result: 'failed' },
  { hash: '0x04', blockNumber: 90, transactionIndex: 0, result: 'success' },
  { hash: '0x05', blockNumber: 80, transactionIndex: 0, result: 'success' },
];

describe('applyQuery', () => {
  it('filters with equal', () => {
    const query: QueryJSON = {
      equal: [{ field: 'result', value: 'success' }],
      limit: 10,
    };
    const results = applyQuery(sampleTransactions, query, txFieldMap);
    assert.equal(results.length, 4);
  });

  it('sorts descending and applies limit', () => {
    const query: QueryJSON = {
      sort: ['blockNumber DESC', 'transactionIndex DESC'],
      limit: 2,
    };
    const results = applyQuery(sampleTransactions, query, txFieldMap);
    assert.deepEqual(
      results.map((tx) => tx.hash),
      ['0x01', '0x02']
    );
  });

  it('handles OR pagination cursor', () => {
    const query: QueryJSON = {
      limit: 10,
      sort: ['blockNumber DESC', 'transactionIndex DESC'],
      or: [
        { lessThan: [{ field: 'blockNumber', value: 90 }] },
        {
          equal: [{ field: 'blockNumber', value: 90 }],
          lessThan: [{ field: 'transactionIndex', value: 1 }],
        },
      ],
    };
    const results = applyQuery(sampleTransactions, query, txFieldMap);
    assert.deepEqual(
      results.map((tx) => tx.hash),
      ['0x04', '0x05']
    );
  });

  it('filters with in on hex field', () => {
    const query: QueryJSON = {
      in: [{ field: 'hash', values: ['01', '0x05'] }],
      limit: 10,
    };
    const results = applyQuery(sampleTransactions, query, txFieldMap);
    assert.deepEqual(
      results.map((tx) => tx.hash),
      ['0x01', '0x05']
    );
  });

  it('filters with like', () => {
    const query: QueryJSON = {
      like: [{ field: 'result', value: 'fail%' }],
      limit: 10,
    };
    const results = applyQuery(sampleTransactions, query, txFieldMap);
    assert.equal(results.length, 1);
    assert.equal(results[0].hash, '0x03');
  });
});
