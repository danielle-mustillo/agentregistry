-- Every Plugin now declares the layout its author wrote it in. Rows stored
-- before spec.type existed get "claude-plugin": it is the only layout the
-- registry read until now, and every harness loads it, so no working
-- Deployment starts failing the deploy-time compatibility check.
--
-- content_hash is deliberately left alone. Reproducing the Go canonical JSON
-- digest in SQL is not possible, and a stale hash only costs one extra
-- generation bump the next time an author re-applies the same intent.
UPDATE plugins
SET spec = jsonb_set(spec, '{type}', '"claude-plugin"'::jsonb),
    generation = generation + 1
WHERE NOT (spec ? 'type');
