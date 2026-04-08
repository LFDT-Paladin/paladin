BEGIN;

ALTER TABLE public_txns ADD "dispatcher" TEXT;
UPDATE public_txns SET "dispatcher" = '';
ALTER TABLE public_txns ALTER COLUMN "dispatcher" SET NOT NULL;

ALTER TABLE chained_private_txns ADD "id" UUID;
UPDATE chained_private_txns SET "id" = '00000000-0000-0000-0000-000000000000';
ALTER TABLE chained_private_txns ALTER COLUMN "id" SET NOT NULL;

COMMIT;
