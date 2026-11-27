# mcp-postgresql

A PostgreSQL MCP (Model Context Protocol) tool that provides LLM agents with database access via three tools:

- `pg_list_target` — lists configured PostgreSQL targets
- `pg_execute_sql` — executes a single SQL statement
- `pg_execute_sql_file` — executes multiple SQL statements in a transaction (supports `dry_run`)

## Configuration

All configuration is provided via environment variables:

| Variable | Description |
|---|---|
| `POSTGRES_TARGETS` | JSON object mapping target names to descriptions |
| `POSTGRES_<TARGET>_DSN` | PostgreSQL DSN for each target named in `POSTGRES_TARGETS` |

Example:

```bash
export POSTGRES_TARGETS='{"app1":"db description of app1","app2":"db description of app2"}'
export POSTGRES_APP1_DSN='postgres://user:pass@localhost:5432/devdb?sslmode=disable'
export POSTGRES_APP2_DSN='postgres://user:pass@remote:5432/proddb?sslmode=verify-full'
```

## Building

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-X main.version=v0.1.0" -o mcp-tool .
```

The resulting binary is a static Linux amd64 executable (Alpine 3 compatible).

## CI/CD

A GitHub Actions workflow (`.github/workflows/release.yml`) is triggered on every push to `main`:

1. **Version increment** — reads the latest `v*` Git tag; if none exists, uses `v0.1.0`; otherwise increments the patch version (e.g. `v0.1.0` → `v0.1.1`).
2. **Build** — compiles a static `linux/amd64` binary with `CGO_ENABLED=0` and injects the version via ldflags.
3. **Package** — creates a `tar.gz` containing the `mcp-tool` binary (executable) and `tool.yaml`.
4. **Release** — publishes a GitHub Release with the version tag and the tar.gz artifact.
