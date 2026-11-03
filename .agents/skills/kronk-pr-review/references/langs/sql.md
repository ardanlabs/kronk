# SQL idioms

**Tools:** `rg`; `sqlfluff` if installed and configured.

## Correctness
Migrations reversible and idempotent; locking on large tables (`ALTER` rewrites, `CREATE INDEX` without `CONCURRENTLY`); destructive changes (drop, rename, type narrowing) without a backfill or expand/contract plan; defaults on large tables; seeds; ordering against application deploys.

## Failure paths
Migrations fail loudly and roll back rather than half-commit; transactions around multi-statement changes.

## Contracts
Types, nullability, constraints, and keys encode the same invariants as the application's types; columns other services or views read.

## Tests
Migration up/down on a copy of production-shaped data; query tests for changed queries.

## Security
String-built queries, missing tenant/owner predicates, grants broader than needed.
