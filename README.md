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
| `pk` | Any | Mark as PRIMARY KEY (overrides default ID→PK behavior) |
| `ref:<table>` | Foreign key field | Set the referenced table (overrides convention-based table name) |
| `del:<rule>` | Foreign key field | Set ON DELETE rule: `cascade`, `setnull`, `restrict`, `noaction` |

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

---

## Naming Conventions

| Source | Convention | Example |
|--------|-----------|---------|
| Entity struct name | PascalCase → snake_case → plural | `User` → `users` |
| Field name | PascalCase → snake_case | `FullName` → `full_name` |
| Primary key | Field named `ID` or annotated `// pk` | `ID` → `id UUID PRIMARY KEY` |
| Foreign key | Field ending in `ID` | `RoleID` → FK to `roles(id)` |
| FK constraint name | `fk_<table>_<column>` | `fk_users_role_id` |
| Unique constraint name | `uq_<table>_<column>` | `uq_users_email` |

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

---

## Generated Migration Files

### Init Migration (CREATE TABLE)

```sql
CREATE TABLE users (
    id UUID PRIMARY KEY,
    email VARCHAR(100) NOT NULL UNIQUE,
    role_id UUID NOT NULL,
    FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE
);
```

```sql
DROP TABLE IF EXISTS users;
```

### Alter Migration (ALTER TABLE)

Up:

```sql
ALTER TABLE users
    DROP CONSTRAINT IF EXISTS fk_users_role_id,
    DROP COLUMN IF EXISTS obsolete,
    ALTER COLUMN email TYPE VARCHAR(100),
    ALTER COLUMN age DROP NOT NULL,
    ADD CONSTRAINT uq_users_age UNIQUE (age),
    ADD COLUMN name VARCHAR(50) NOT NULL,
    ADD CONSTRAINT fk_users_role_id FOREIGN KEY (role_id) REFERENCES teams(id) ON DELETE RESTRICT;
```

Down (reverses the alter):

```sql
ALTER TABLE users
    DROP CONSTRAINT IF EXISTS fk_users_role_id,
    DROP COLUMN IF EXISTS name,
    DROP CONSTRAINT IF EXISTS uq_users_age,
    ALTER COLUMN age SET NOT NULL,
    ALTER COLUMN email TYPE TEXT,
    ADD COLUMN obsolete TEXT NOT NULL,
    ADD CONSTRAINT fk_users_role_id FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE;
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

1. **Read migration history** — Parse migration SQL files to reconstruct current schema state (see `internal/replay.go`)
2. **Parse current entities** — Read Go struct files via `go/ast` (see `internal/schema.go`)
3. **Diff** — Compare columns and foreign keys, categorizing changes as added/removed/changed (see `internal/diff.go`)
4. **Generate SQL** — Produce ALTER TABLE statements from the diff (see `internal/generate.go`)
5. **Delegate** — Write SQL files and let `golang-migrate` handle execution

### Column Rename Warning

When a column is removed and another column is added with the same data type, Wigrate prints a warning to stderr:
```
warning: column "old_name" removed and "new_name" added with same type "TEXT" — if this is a rename, data will be lost
```
This is a safety signal — the diff engine cannot distinguish a rename from a drop+add.

### Limitations (v1)

- Primary key changes (adding, removing, or changing a PK column) are intentionally blocked in alter migrations
- Supported types: `string`, `int`, `int32`, `int64`, `bool`, `float32`, `float64`, `time.Time`, `uuid.UUID`
- Only PostgreSQL is supported as a target
- No default value support in the inline DSL

---

## Architecture

```
wigrate gen
  │
  ├── discover modules (module/<name>/)
  ├── parse entity structs (go/ast) → tableSchema
  ├── replay past migrations → existing tableSchema
  ├── diff (existing vs current) → schemaDiff
  ├── generate SQL (CREATE/ALTER TABLE)
  └── delegate to golang-migrate CLI

wigrate up/down/status
  │
  ├── load DB config from env / .env
  ├── discover modules
  └── delegate to migrate CLI per module
```

The tool has zero runtime dependencies (Go stdlib only). The only external requirement is the `migrate` CLI binary on PATH.
