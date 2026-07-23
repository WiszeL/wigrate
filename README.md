# Wigrate — Schema Migration Generator for Go Entities

Wigrate reads Go entity structs via AST parsing, diffs them against replay of past migrations, generates PostgreSQL migration SQL, and delegates execution to the [`golang-migrate`](https://github.com/golang-migrate/migrate) CLI.

Each module in a modular monolith owns its schema under `module/<name>/migration/` with its own `golang-migrate` tracking table (`schema_migrations_<name>`).

---

## Quick Start

```bash
# Install
go install github.com/wiszel/wigrate/cmd/wigrate@latest

# Generate migrations from entity structs
wigrate gen

# Apply pending migrations
wigrate up
```

---

## Commands

### `wigrate gen`

Discover modules, parse entity structs, diff against migration history, and generate SQL migration files.

```bash
wigrate gen                         # all modules
wigrate gen -m=iam                  # single module
wigrate gen -o                      # overwrite latest migration (all modules)
wigrate gen -o -m=iam               # overwrite latest migration for one module
wigrate gen --dry-run               # print generated SQL without writing files
wigrate gen --modules-dir=my_mods   # use custom modules directory
```

**Flags:**
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--overwrite` | `-o` | `false` | Overwrite the latest migration instead of creating a new alter |
| `--module` | `-m` | `""` | Generate only for this module (empty = all) |
| `--modules-dir` | | `"module"` | Base directory for modules (absolute or relative to project root) |
| `--dry-run` | | `false` | Print what would be generated without writing files or calling `migrate` CLI |

### `wigrate up`

Apply pending migrations via `golang-migrate`.

```bash
wigrate up                # all modules
wigrate up -m=iam         # single module
wigrate up --modules-dir=my_mods
```

### `wigrate down <steps>`

Roll back migrations by step count.

```bash
wigrate down 1            # rollback one step in all modules
wigrate down 3 -m=iam     # rollback three steps in one module
```

`down` requires a step count so rollback is always explicit.

### `wigrate status`

Show current migration version per module.

```bash
wigrate status            # all modules
wigrate status -m=iam     # single module
```

Delegates to `migrate version` which prints the current migration version and dirty state.

---

## Entity Annotations (Inline Comment DSL)

Fields in entity structs carry database schema information through Go comments:

```go
type User struct {
    ID       uuid.UUID                       // → id UUID PRIMARY KEY (auto)
    Email    string       // 100 null unique  // → email VARCHAR(100) UNIQUE
    Username string       // unique           // → username TEXT UNIQUE
    Age      int          // null             // → age INTEGER
    RoleID   uuid.UUID    // ref:roles        // → FK to roles(id) (auto column name)
    OwnerID  uuid.UUID    // ref:teams        // → FK to teams(id) (custom ref table)
    CustomPK uuid.UUID    // pk               // → custom_pk UUID PRIMARY KEY
    Bio      *string                          // → bio TEXT (nullable via pointer)
}
```

### Annotation Reference

| Annotation | Applies To | Effect |
|------------|-----------|--------|
| `<number>` | `string` | Set VARCHAR length. Without it, `string` → `TEXT` |
| `null` | Any | Column is nullable (omit NOT NULL) |
| `unique` | Any | Add UNIQUE constraint |
| `unique:<group>` | Any | Group two or more fields into one composite UNIQUE constraint |
| `index` | Any | Add a plain (non-unique) index, emitted as a standalone `CREATE INDEX` statement |
| `index:<group>` | Any | Group two or more fields into one composite index |
| `trgm` | `string` | Add a GIN trigram index for fuzzy/`ILIKE '%x%'` search |
| `pk` | Any | Mark as PRIMARY KEY (overrides default ID→PK behavior). Two or more `pk` fields form a composite PRIMARY KEY |
| `ref:<table>` | Foreign key field | Set the referenced table (overrides convention-based table name) |
| `del:<rule>` | Foreign key field | Set ON DELETE rule: `cascade`, `setnull`, `restrict`, `noaction` |

### Composite Keys

Mark two or more fields `pk` for a composite PRIMARY KEY, or share the same `unique:<group>`
label for a composite UNIQUE constraint. Composite FK is not supported.

```go
type Membership struct {
    TeamID uuid.UUID // pk
    UserID uuid.UUID // pk
    RoleID uuid.UUID // unique:role del:cascade
    Label  string    // unique:role
}
```

```sql
CREATE TABLE memberships (
    team_id UUID NOT NULL,
    user_id UUID NOT NULL,
    role_id UUID NOT NULL,
    label TEXT NOT NULL,
    PRIMARY KEY (team_id, user_id),
    CONSTRAINT uq_memberships_role_id_label UNIQUE (role_id, label),
    CONSTRAINT fk_memberships_role_id FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE
);
```

Composite UNIQUE can be added/removed via alter migrations like any other constraint.
Composite PRIMARY KEY, like single-column PK, can only be set at CREATE TABLE time —
changing it later is blocked in alter migrations (see Limitations).

### Indexes

`index` (single-column) and `index:<group>` (composite, same grouping rule as
`unique:<group>`) add a plain, non-unique index. Unlike UNIQUE, an index is not a
table constraint — it's always emitted as its own `CREATE INDEX` / `DROP INDEX IF EXISTS`
statement, never inline in `CREATE TABLE` or nested inside `ALTER TABLE`:

```go
Email string // 100 index
```

```sql
CREATE TABLE users (
    email VARCHAR(100) NOT NULL
);
CREATE INDEX idx_users_email ON users (email);
```

Indexes can be added/removed freely via alter migrations.

### Trigram Indexes

`trgm` adds a GIN trigram index on a `string` field, for fuzzy/`ILIKE '%x%'` search.
Rejected on non-string fields. Like `index`, it's always a standalone statement:

```go
Username string // 25 unique trgm
```

```sql
CREATE TABLE users (
    username VARCHAR(25) NOT NULL UNIQUE
);
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX idx_users_username_trgm ON users USING GIN (username gin_trgm_ops);
```

`CREATE EXTENSION IF NOT EXISTS pg_trgm` is idempotent and DB-global — emitted once per
migration file (not once per column), regardless of how many `trgm` fields it contains.

Down migration only drops the index, never the extension — other columns/tables may
still depend on it:

```sql
DROP INDEX IF EXISTS idx_users_username_trgm;
```

### Field Descriptions

Put human-readable descriptions in the comment **above** the field; inline trailing
comments are DSL-only and never parsed as free text:

```go
// DPoP key thumbprint bound at login
Thumbprint string // 100 unique
```

### Pointer Nullability

Pointer types (`*string`, `*int`, `*uuid.UUID`, etc.) default to nullable without needing `// null`. Non-pointer types default to NOT NULL. Explicit `// null` overrides on non-pointer types also works.

### Foreign Key Detection

Any non-PK field ending in `ID` is automatically treated as a foreign key. The referenced table is derived from the field name by stripping `ID`, converting to snake_case, and pluralizing. Override with `ref:<table>`.

Example:
- `RoleID uuid.UUID` → FK to `roles(id)` (convention)
- `OwnerID uuid.UUID // ref:teams` → FK to `teams(id)` (explicit)

### Delete Rules

```go
RoleID uuid.UUID // ref:roles del:cascade    → ON DELETE CASCADE
RoleID uuid.UUID // ref:roles del:setnull    → ON DELETE SET NULL (field must be nullable)
RoleID uuid.UUID // ref:roles del:restrict   → ON DELETE RESTRICT
RoleID uuid.UUID // ref:roles del:noaction   → ON DELETE NO ACTION
```

### Enum Fields

Give a field a named Go type with a `const` block, defined anywhere in the
entity's directory, and wigrate treats it as an enum automatically — no DSL
token needed.

```go
// payment_status.go
type PaymentStatus string

const (
    PaymentPending PaymentStatus = "pending"
    PaymentPaid    PaymentStatus = "paid"
    PaymentFailed  PaymentStatus = "failed"
)
```

```go
// payment.go
type Payment struct {
    ID     uuid.UUID
    Status PaymentStatus
}
```

```sql
CREATE TABLE payments (
    id UUID PRIMARY KEY,
    status VARCHAR(7) NOT NULL,
    CONSTRAINT chk_payments_status CHECK (status IN ('failed','paid','pending'))
);
```

- **String enum** → `VARCHAR(n)` sized to the longest label, with `CHECK (col
  IN ('a','b'))`.
- **Int enum** (`iota` or explicit values) → `INTEGER`/`BIGINT` with `CHECK
  (col IN (0,1,2))`.
- Reordering the `const` block never triggers a migration.
- Adding/removing a value generates an alter migration that updates the
  `CHECK`. Removing a value also prints a warning — existing rows holding it
  will fail the migration.
- Only bare `iota` and literal values are supported (no `1 << iota`).

### Value Objects (Custom Struct Fields)

Give a field a type that's a named struct defined anywhere in the entity's
directory, and — provided that struct declares no primary key of its own —
wigrate treats it as a **value object** and flattens its fields into the
parent table, prefixed by the field name:

```go
// payment.go
type Payment struct {
    ID   uuid.UUID
    Cust Customer
}
```

```go
// customer.go
type Customer struct {
    Name  string
    Email string
}
```

```sql
CREATE TABLE payments (
    id UUID PRIMARY KEY,
    cust_name TEXT NOT NULL,
    cust_email TEXT NOT NULL
);
```

- **Prefix is the field name**, not the struct type — `Buyer Customer` and
  `Seller Customer` on the same entity produce distinct `buyer_*`/`seller_*`
  columns, never a collision.
- **Recursive**: a value object can itself hold another value-object field,
  flattened to any depth (`Buyer.Address.City` → `buyer_address_city`). A
  struct that references itself, directly or through a cycle, is an error.
- **Full DSL support inside**: nested fields keep every annotation — `null`,
  `unique`, `index`, `trgm`, FK detection, enum detection — exactly as if
  written inline on the parent, just with the prefix applied. `unique:<group>`
  and `index:<group>` labels are scoped to the value object, so the same group
  name reused across two value-object fields (or at the parent level) never
  merges into one wrong constraint.
- **No DSL on the struct-typed field itself** — `Cust Customer // null` is an
  error; nullability, uniqueness, etc. belong on the leaf fields inside the
  value object.
- **The struct must have no primary key** (no bare `ID` field, no `pk`
  annotation) — a value object owns no identity, so a would-be `cust_id
  PRIMARY KEY` on the parent table is rejected as an error. A struct **with**
  a primary key is a normal entity, not a value object, and referencing it as
  a field errors rather than silently flattening it.
- **Same-dir resolution only**, like FK and enum detection — a field typed as
  a struct from another package is not flattened; it stays an "unsupported
  field type" error.
- A struct that's used only as a value object (never has its own primary key)
  never gets a table of its own, the same way an enum's `type X string` +
  `const` block never does — no `.wigrateignore` entry needed.

---

## Naming Conventions

| Source | Convention | Example |
|--------|-----------|---------|
| Entity struct name | PascalCase → snake_case → plural | `User` → `users` |
| Field name | PascalCase → snake_case | `FullName` → `full_name` |
| Primary key | Field named `ID` or annotated `// pk` | `ID` → `id UUID PRIMARY KEY` |
| Foreign key | Field ending in `ID` | `RoleID` → FK to `roles(id)` |
| FK constraint name | `fk_<table>_<refTable>` | `fk_users_roles` |
| Unique constraint name | `uq_<table>_<column>` | `uq_users_email` |
| Index name | `idx_<table>_<column>` | `idx_users_email` |
| Enum CHECK constraint name | `chk_<table>_<column>` | `chk_payments_status` |

### Pluralization Rules

- Ends in consonant + `y` → `-ies` (e.g. `category` → `categories`)
- Ends in `s`, `x`, `z`, `ch`, `sh` → add `-es` (e.g. `address` → `addresses`)
- Otherwise → add `-s` (e.g. `user` → `users`)

---

## Module Structure

```
project-root/
├── go.mod
├── module/
│   ├── iam/
│   │   ├── internal/domain/entity/
│   │   │   ├── user.go
│   │   │   └── role.go
│   │   └── migration/
│   │       ├── 000001_init_user.up.sql
│   │       ├── 000001_init_user.down.sql
│   │       ├── 000002_alter_name_role_id_user.up.sql
│   │       └── 000002_alter_name_role_id_user.down.sql
│   └── billing/
│       ├── internal/domain/entity/
│       │   └── payment.go
│       └── migration/
│           └── ...
└── .env
```

Each entity file must contain a struct whose name matches the file name in PascalCase (e.g. `user.go` → `type User struct`).

The modules directory is configurable with `--modules-dir` flag (default: `"module"`).

### Excluding Entities from Migration

Not every entity in `internal/domain/entity/` is backed by this Postgres schema (e.g. a
Redis-only `Session`). List its name (filename without `.go`) in a `.wigrateignore` file
inside the module's `migration/` directory to skip it entirely — kept infra-side so the
domain entity file itself carries no dependency on the migration tool:

```
# module/iam/migration/.wigrateignore
session
```

This is for real entities that just aren't backed by Postgres. A file that declares no
struct at all — like an enum's `type X string` + `const` block sitting next to the
entity that uses it — is a support file, not an entity, and is skipped automatically;
it does not need a `.wigrateignore` entry.

---

## Generated Migration Files

### Init Migration (CREATE TABLE)

```sql
CREATE TABLE users (
    id UUID PRIMARY KEY,
    email VARCHAR(100) NOT NULL UNIQUE,
    role_id UUID NOT NULL,
    CONSTRAINT fk_users_roles FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE
);
```

```sql
DROP TABLE IF EXISTS users;
```

### Alter Migration (ALTER TABLE)

Up:

```sql
ALTER TABLE users
    DROP CONSTRAINT IF EXISTS fk_users_roles,
    DROP COLUMN IF EXISTS obsolete,
    ALTER COLUMN email TYPE VARCHAR(100),
    ALTER COLUMN age DROP NOT NULL,
    ADD CONSTRAINT uq_users_age UNIQUE (age),
    ADD COLUMN name VARCHAR(50) NOT NULL,
    ADD CONSTRAINT fk_users_roles FOREIGN KEY (role_id) REFERENCES teams(id) ON DELETE RESTRICT;
```

Down (reverses the alter):

```sql
ALTER TABLE users
    DROP CONSTRAINT IF EXISTS fk_users_roles,
    DROP COLUMN IF EXISTS name,
    DROP CONSTRAINT IF EXISTS uq_users_age,
    ALTER COLUMN age SET NOT NULL,
    ALTER COLUMN email TYPE TEXT,
    ADD COLUMN obsolete TEXT NOT NULL,
    ADD CONSTRAINT fk_users_roles FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE;
```

---

## Database Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DB_HOST` | Yes | — | Postgres host |
| `DB_PORT` | Yes | — | Postgres port |
| `DB_NAME` | Yes | — | Database name |
| `DB_USER` | Yes | — | Database user |
| `DB_PASSWORD` | Yes | — | Database password |
| `DB_SSLMODE` | No | `disable` | Postgres SSL mode |

Wigrate loads `.env` from the project root when it exists. Process environment variables take priority over `.env` values.

### Per-Module Tracking Tables

Each module gets its own `golang-migrate` tracking table to avoid version collisions:

```
module/iam/migration      → schema_migrations_iam
module/billing/migration  → schema_migrations_billing
```

This is passed through the Postgres URL as `x-migrations-table=schema_migrations_<module_name>`.

---

## Schema Diff Algorithm

1. **Read migration history** — Parse migration SQL files to reconstruct current schema state (see `internal/replay/`)
2. **Parse current entities** — Read Go struct files via `go/ast` (see `internal/schema/`)
3. **Diff** — Compare columns and foreign keys, categorizing changes as added/removed/changed (see `internal/diff/`)
4. **Generate SQL** — Produce ALTER TABLE statements from the diff (see `internal/sqlgen/`, orchestrated by `internal/migration/`)
5. **Delegate** — Write SQL files and let `golang-migrate` handle execution

### Column Rename Warning

When a column is removed and another column is added with the same data type, Wigrate prints a warning to stderr:
```
warning: column "old_name" removed and "new_name" added with same type "TEXT" — if this is a rename, data will be lost
```
This is a safety signal — the diff engine cannot distinguish a rename from a drop+add.

### Limitations (v1)

- Primary key changes (adding, removing, or changing PK columns — single or composite) are intentionally blocked in alter migrations
- Composite foreign keys are not supported
- Supported types: `string`, `int`, `int32`, `int64`, `bool`, `float32`, `float64`, `time.Time`, `uuid.UUID`, plus same-dir named enum and value-object structs
- Only PostgreSQL is supported as a target
- No default value support in the inline DSL

---

## Architecture

```
wigrate gen
  │
  ├── discover modules (module/<name>/)          — internal/discover
  ├── parse entity structs (go/ast) → schema.Table — internal/schema
  ├── replay past migrations → existing schema.Table — internal/replay
  ├── diff (existing vs current) → diff.Result    — internal/diff
  ├── generate SQL (CREATE/ALTER TABLE)           — internal/sqlgen, internal/migration
  └── delegate to golang-migrate CLI

wigrate up/down/status
  │
  ├── load DB config from env / .env
  ├── discover modules
  └── delegate to migrate CLI per module
```

The tool has zero runtime dependencies (Go stdlib only). The only external requirement is the `migrate` CLI binary on PATH.
