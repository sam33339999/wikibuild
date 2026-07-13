# WikiBuild MCP (stdio)

Agent-facing article tools over the [Model Context Protocol](https://modelcontextprotocol.io) (stdio).

## Prerequisites

1. PostgreSQL migrated (`make migrate-up`).
2. `.env` with `DATABASE_URL` and a non-empty:

```bash
WIKIBUILD_MCP_TOKEN=your-long-random-secret
```

Empty token → `wikibuild mcp` refuses to start (fail closed).

## Run

```bash
# from repo root (loads .env)
go run ./cmd/wikibuild mcp
# or
./wikibuild mcp
```

Stdin/stdout is the MCP transport — do not pipe logs to stdout.

## Tools

| Tool | Purpose |
|------|---------|
| `list_articles` | Filter by `status`, `visibility`, `q`; `limit`/`offset` |
| `get_article` | By `id` or `slug` (includes body) |
| `create_article` | Markdown; **defaults `draft` + `private`**; optional SEO fields |
| `update_article` | Patch by `id` (title/body/tags/SEO/…) |
| `set_article_status` | `draft` \| `published` (+ optional `publish_at` RFC3339) |
| `set_article_visibility` | `public` \| `protected` \| `private` |

Responses never include password hashes.

## Example agent config (Cursor / Claude Desktop)

```json
{
  "mcpServers": {
    "wikibuild": {
      "command": "/path/to/wikibuild",
      "args": ["mcp"],
      "env": {
        "DATABASE_URL": "postgres://wikibuild:wikibuild@localhost:5432/wikibuild?sslmode=disable",
        "WIKIBUILD_MCP_TOKEN": "your-long-random-secret",
        "WIKIBUILD_ADMIN_USER": "admin",
        "WIKIBUILD_ADMIN_PASS": "changeme",
        "WIKIBUILD_SESSION_SECRET": "0123456789abcdef0123456789abcdef"
      }
    }
  }
}
```

(`config.Load` still requires the usual app env vars even for MCP.)

## Safety

- Prefer local agent launches; do not expose stdio MCP on a public network.
- Create defaults are **draft + private** so agents cannot silently publish public posts.
