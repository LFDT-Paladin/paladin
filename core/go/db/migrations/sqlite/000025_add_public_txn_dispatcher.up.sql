ALTER TABLE public_txns ADD "dispatcher" TEXT;
UPDATE public_txns SET "dispatcher" = '';
ALTER TABLE public_txns ALTER COLUMN "dispatcher" SET NOT NULL;
