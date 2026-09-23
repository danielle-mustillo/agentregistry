-- Drop the format declaration. A pre-017 binary has no spec.type field, so it
-- would discard the value on the next write anyway. This removes an author's
-- deliberate "agent-plugins" too: that value cannot survive a downgrade, and
-- leaving it behind would make a plugin the old binary cannot describe.
UPDATE plugins
SET spec = spec - 'type'
WHERE spec ? 'type';
