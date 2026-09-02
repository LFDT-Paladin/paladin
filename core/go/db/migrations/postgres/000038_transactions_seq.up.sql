-- when transactions are insterted in batches, created is not unique. An incrementing
-- seq column ensures that we can order within a batch, which is necessary for correct
-- submission ordering of chained transactions.
ALTER TABLE transactions ADD "seq" BIGINT GENERATED ALWAYS AS IDENTITY;
CREATE UNIQUE INDEX transactions_seq ON transactions("seq");
