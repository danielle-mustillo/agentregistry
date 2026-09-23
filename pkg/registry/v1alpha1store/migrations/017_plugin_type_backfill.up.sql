-- Every Plugin now declares the layout its author wrote it in. Rows stored
-- before spec.type existed get "claude-plugin": it is the only layout the
-- registry read until now, and every harness loads it, so no working
-- Deployment starts failing the deploy-time compatibility check.
--
-- generation is deliberately NOT bumped. The backfill changes nothing the
-- Plugin controller resolves, and a bump would put generation ahead of
-- status.observedGeneration on every row at once. That hides each plugin from
-- marketplace.json (pkg/pluginmarketplace.FromPlugin skips a plugin whose
-- status has not caught up) and re-resolves every git source on upgrade. The
-- spec content change alone moves each Deployment's desired fingerprint, which
-- is the redeploy this release plans for.
--
-- content_hash is left alone too. The Go canonical-JSON digest cannot be
-- reproduced in SQL. The cost is that the first re-apply of otherwise
-- unchanged intent counts as a real write instead of an UpsertNoOp.
UPDATE plugins
SET spec = jsonb_set(spec, '{type}', '"claude-plugin"'::jsonb)
WHERE NOT (spec ? 'type');
