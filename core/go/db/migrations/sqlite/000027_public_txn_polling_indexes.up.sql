CREATE INDEX public_txns_poll_idx
ON public_txns ("from", "dispatcher", "pub_txn_id")
WHERE suspended IS FALSE;
