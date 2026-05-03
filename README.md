
### Migration

Wigrate keeps database changes module-owned while still delegating actual migration execution to [`golang-migrate`](https://github.com/golang-migrate/migrate). It does not try to replace `golang-migrate`; it only discovers modules, generates SQL files from entities, builds Postgres connection URLs from the app database environment, and calls the `migrate` CLI.

Each module owns its migrations under its own `migration` directory:

```text
module/
  iam/
    migration/
  billing/
    migration/
```

This keeps schema changes close to the business area that owns them and supports the Modular Monolith direction of Wibee.

#### Generate migrations

Generate migrations for all modules:

```bash
go run ./cmd gen
```

Generate migrations for one module:

```bash
go run ./cmd gen -m=iam
```

Overwrite the latest migration for each affected entity:

```bash
go run ./cmd gen -o
go run ./cmd gen -o -m=iam
```

Wigrate reads entities from:

```text
module/<module_name>/internal/domain/entity
```

and writes generated migrations to:

```text
module/<module_name>/migration
```

#### Apply and rollback migrations

Apply migrations for all modules:

```bash
go run ./cmd up
```

Apply migrations for one module:

```bash
go run ./cmd up -m=iam
```

Rollback migrations by step count:

```bash
go run ./cmd down 1
go run ./cmd down 1 -m=iam
```

`down` requires a step count so a rollback is always explicit.

#### Database environment

Wigrate targets Postgres and builds its database URL from the same `DB_*` variables the application should use. This keeps database configuration in one source of truth instead of duplicating a separate `DATABASE_URL`.

Required:

```env
DB_HOST=localhost
DB_PORT=5432
DB_NAME=wibee
DB_USER=postgres
DB_PASSWORD=secret
```

Optional:

```env
DB_SSLMODE=disable
```

If `DB_SSLMODE` is empty, Wigrate defaults it to `disable`.

Wigrate loads `.env` from the project root when it exists. Existing process environment variables take priority over `.env` values, so CI or shell overrides still work.

#### Module migration tables

Each module uses its own `golang-migrate` tracking table to avoid version collisions between module-owned migration folders.

For example:

```text
module/iam/migration      -> schema_migrations_iam
module/billing/migration  -> schema_migrations_billing
```

Wigrate passes this through Postgres URL query parameter:

```text
x-migrations-table=schema_migrations_<module_name>
```

After installing the command as `wigrate`, the same commands can be used without `go run ./cmd`:

```bash
wigrate gen -m=iam
wigrate up
wigrate down 1 -m=iam
```

### 2. Mapping

For persistence mapping, a main domain entity should normally have a clear main table. For example, a `User` entity would usually map to a `users` table, and a `Payment` entity would usually map to a `payments` table.

Relationships between entities should be represented by related entity IDs by default. The database can enforce those relationships with foreign keys, while the domain entity keeps a clear reference such as `UserID`, `PaymentID`, or another module-owned identifier.
