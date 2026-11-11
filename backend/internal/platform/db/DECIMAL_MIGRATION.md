# Expand quantity storage without changing visible units

Migration 00013 changes persisted quantity columns to NUMERIC(16,3) and rebuilds
the deferred inventory-total trigger with numeric local variables. Existing
whole-unit values retain their meaning. No endpoint starts accepting fractional
quantities in this schema-only slice; application activation follows separately.

Rollback refuses fractional historical quantities rather than truncating them.
Take a database backup and verify migrations on a restored copy before rollout.
Once later application changes write fractions, do not deploy integer-only
readers or force a lossy downgrade.

`TestDecimalSchemaPreservesLegacyUnitsAndRejectsLossyRollback` creates a private
test database, checks legacy units and rollback rejection, and drops that database.
