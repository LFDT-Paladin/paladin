ALTER TABLE public_txns ADD "dispatcher" TEXT;
UPDATE public_txns SET "dispatcher" = '';
