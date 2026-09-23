DO $$ BEGIN
  RAISE EXCEPTION 'migration 017_plugin_type_backfill is not reversible (up-only)';
END $$;
