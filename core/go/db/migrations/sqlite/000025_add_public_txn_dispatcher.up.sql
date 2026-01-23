ALTER TABLE public_txns ADD "dispatcher" TEXT;
UPDATE public_txns SET "dispatcher" = '';
ALTER TABLE chained_private_txns ADD "id" UUID;
-- cannot set NOT NULL constraint on SQLite. Not critical to have.