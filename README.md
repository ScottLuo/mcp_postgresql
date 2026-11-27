# mcp-postgresql

A Go-based PostgreSQL MCP (Model Context Protocol) Server that lets LLM agents run SQL against one or more pre-configured PostgreSQL databases — without ever seeing the connection details (host, user, password).

The server speaks the MCP standard over **stdio** (JSON-RPC) and exposes a small set of tools for listing targets and executing SQL.

## Features

- **Multi-target configuration** — define as many PostgreSQL targets as you need, each identified by a name, with its DSN read from a separate environment variable.
- **Hidden connection details** — the LLM never sees the DSN; the server maintains an internal `pgxpool` connection pool per target and only hands the LLM a `target_name`.
- **Two execution modes** —
  - a single SQL statement (`pg_execute_sql`), and
  - a multi-statement file executed atomically in a transaction (`pg_execute_sql_file`), with an optional `dry_run` mode that rolls back after execution.
- **Result truncation** — `limit_rows` (default `1000`) caps the number of returned rows to protect the LLM context window from exploding.
- **PG error code reporting** — PostgreSQL error codes and messages (e.g. `42501` Insufficient Privilege) are surfaced to the LLM to help it reason about syntax, permission, and constraint errors.
- **Runaway-query protection** — a `30s` `statement_timeout` is applied to connections so an unbounded query cannot hang the server.

## Tools

### `pg_list_target`

List all configured PostgreSQL targets and their descriptions.

| Parameter | Type | Required | Description |
|---|---|---|---|
| *(none)* | — | — | No input parameters. |

Returns:

```json
{"targets": [{"name": "app1", "description": "dev db for app1"}]}
```

### `pg_execute_sql`

Execute a single SQL statement against a target.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `target_name` | string | yes | The name of the PostgreSQL target to use. |
| `sql` | string | yes | The SQL statement to execute. |
| `limit_rows` | int | no | Maximum number of rows to return (default `1000`). |

Returns a JSON result (`columns`, `rows`, `row_count`, `truncated`) for `SELECT` statements, or affected-row information for write statements. On failure it returns:

```json
{"error": "<pg error code + message>"}
```

### `pg_execute_sql_file`

Execute all statements in a SQL file inside a single `BEGIN` / `COMMIT` / `ROLLBACK` transaction.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `target_name` | string | yes | The name of the PostgreSQL target to use. |
| `file_name` | string | yes | The path to the SQL file to execute. |
| `dry_run` | bool | no | If `true`, the file is executed but the transaction is rolled back afterwards (nothing is persisted). |

All statements are split on statement boundaries (aware of dollar-quoting, comments, and quoted strings) and executed in one transaction: on success it is committed, on error it is rolled back so the operation is atomic. When `dry_run` is `true`, the transaction is rolled back even on success.

## Configuration

All runtime configuration is read from **environment variables** at startup. Missing required variables cause the server to print an error to stderr and exit.

| Variable | Required | Description |
|---|---|---|
| `POSTGRES_TARGETS` | yes | JSON object mapping target name → description. |
| `POSTGRES_<TARGET>_DSN` | yes (one per target) | DSN for a single target. The target name is upper-cased in the variable name. |
| `LOG_LEVEL` | no | Log level. Default `INFO`. |

### Example

```bash
export POSTGRES_TARGETS='{"app1":"dev db for app1","app2":"prod db for app2"}'
export POSTGRES_APP1_DSN='postgres://mcp_agent:secret@localhost:5432/devdb?sslmode=disable'
export POSTGRES_APP2_DSN='postgres://mcp_agent:secret@remote_db:5432/proddb?sslmode=verify-full'
export LOG_LEVEL=INFO
```

> **Note:** For target `app1` the DSN is read from `POSTGRES_APP1_DSN` (target name upper-cased).

## Build

### Local development

```bash
go build -o bin/mcp-tool-binary .
```

### Release (Linux amd64, Alpine 3 compatible)

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/mcp-tool-binary .
```

The resulting binary is a static executable (~17 MB) with no CGO dependencies, so it runs inside an **Alpine Linux 3** container.

## CI/CD Release Packaging

The release process (push to `main` → release → Linux amd64 package) must produce a package that contains exactly:

1. The compiled binary `bin/mcp-tool-binary` (with execute permission).
2. The `tool.yaml` contract file.

```bash
# 1. Build the static binary
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/mcp-tool-binary .

# 2. Ensure the binary is executable
chmod +x bin/mcp-tool-binary

# 3. Package the release
tar -czf mcp-postgresql-linux-amd64.tar.gz bin/mcp-tool-binary tool.yaml
```

Requirements:

- Target platform: `linux/amd64`.
- Built with `CGO_ENABLED=0` for a static binary.
- The binary must run on **Alpine Linux 3**.
- The binary must carry execute permission.

## Security & Design Notes

- **Least-privilege DB account** — the server connects with a dedicated account (e.g. `mcp_agent`) that is granted only the privileges it needs (`SELECT`, `INSERT`, `UPDATE`, `DELETE`, and optionally `CREATE ROLE`, `GRANT`, `VACUUM`). The DSN — and therefore the credentials — never leak to the LLM.
- **Stateless sessions** — each tool call is kept idempotent where possible. `pg_execute_sql_file` manages its own transaction boundary with explicit `BEGIN` / `COMMIT` / `ROLLBACK` to guarantee atomicity.
- **Result truncation** — `limit_rows` (default `1000`) caps row output so a large `SELECT` cannot overflow the LLM context.
- **Timeout** — a `30s` `statement_timeout` is applied to connections to prevent runaway queries generated by the LLM.

## Project Structure

| File | Purpose |
|---|---|
| `main.go` | Entry point: loads config, initializes pools, registers MCP tools, runs the stdio server, handles graceful shutdown. |
| `config.go` | Reads and validates configuration from environment variables. |
| `pool.go` | Per-target connection pool management, single- and transactional execution, result collection and serialization. |
| `sql.go` | Statement-boundary splitting (dollar-quoting, comments, and quote aware). |
| `tools.go` | MCP tool handlers (`pg_list_target`, `pg_execute_sql`, `pg_execute_sql_file`). |
| `tool.yaml` | MCP runtime contract (language, entrypoint, env, timeout). |
