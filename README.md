# Benchmarking Platform

A production-grade, multi-tenant benchmarking platform for AI agents.

## Quick Start

### Using Docker Compose (Recommended)

```bash
# Build and start all services (Development)
docker-compose up --build

# Or run in detached mode
docker-compose up -d --build
```

This will start:
- **PostgreSQL** on port `5432`
- **Go API** on port `8080`

### Verify Services

```bash
# Check Go API health
curl http://localhost:8080/health
```

### Database Migrations

The platform includes an automated migration runner. Place SQL migration files in `server_go/migrations/` (naming convention: `XXX_description.sql`). They are automatically applied on server startup.

- **Initial Schema**: `server_go/migrations/001_initial_schema.sql` contains the baseline database structure.

### Docker Configuration

The project supports two main environments:

- **Development** (`docker-compose.yml`):
  - Hot-reloading for Frontend (Vite)
  - Debug ports exposed
  - Local volume mounts

- **Production** (`docker-compose.prod.yml`):
  - Optimized production builds (Nginx serving static files)
  - Secure proxy configuration
  - Minimized container images

### Maintenance & Reset

Use the included `reset.sh` script for environment management:

```bash
# Default: Resets Database only (Fast)
./reset.sh

# Soft Reset: Rebuilds containers, preserves DB data
./reset.sh --soft-reset

# Hard Reset: Wipes DB volume, rebuilds everything (Fresh Start)
./reset.sh --hard-reset

# Deploy to Production
./reset.sh --prod
```

## API Architecture

This platform uses a **WebSocket-first architecture**. All real-time operations (agents, question sets, runs, evaluations, stats) are handled via WebSocket messages.

### REST Endpoints (Minimal)

Only essential auth endpoints use REST:

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check |
| POST | `/auth/register` | Legacy registration (Dev only) |
| POST | `/auth/login` | Legacy login (Dev only) |
| POST | `/auth/bootstrap-admin` | Create initial admin |
| GET | `/auth/check-admin` | Check if admin exists |
| GET | `/auth/me` | Get current user (protected) |
| POST | `/auth/refresh` | Refresh JWT token (protected) |
| POST | `/auth/logout` | Logout (protected) |
| POST | `/auth/join-organization` | Join org via invite (protected) |
| POST | `/auth/select-organization` | Switch organization (protected) |

### WebSocket API

| Endpoint | Description |
|----------|-------------|
| `GET /ws?token=<jwt>` | Main WebSocket connection |

WebSocket message map (current implementation):

All messages use the same envelope format:

```json
{
  "type": "REQ_SYNC_STATE",
  "correlation_id": "uuid-or-client-id",
  "payload": {}
}
```

Current routed inbound message types (`server_go/api/ws_handlers.go`):

- **Commands**: `CMD_START_RUN`, `CMD_CANCEL_RUN`, `CMD_RUN_EVALUATORS`, `CMD_RERUN_TASK`, `CMD_SEED_HISTORICAL_RUN`, `CMD_ADMIN_RECALCULATE_STATS`, `CMD_ADMIN_FORCE_LOGOUT`, `CMD_ADMIN_START_MAINTENANCE`
- **Core / Stats / Runs**: `REQ_SYNC_STATE`, `REQ_GET_RUN_DETAILS`, `REQ_CREATE_EVALUATION`, `REQ_GET_SPY_PAYLOAD`, `REQ_GET_WORKSPACE_STATS`, `REQ_GET_ORG_STATS`, `REQ_GET_GLOBAL_STATS`, `REQ_GET_MANAGER_STATS`, `REQ_GET_MANAGER_USERS`, `REQ_GET_RUN_LITE`, `REQ_GET_LATEST_RUN_BY_QS`, `REQ_GET_RESULT_DETAILS`, `REQ_GET_RETRY_STATUS`, `REQ_GET_WORKSPACE_RUNS`, `REQ_GET_WORKSPACE_CLIENTS`, `REQ_DELETE_RUN`, `REQ_DELETE_ALL_RUNS`, `REQ_CHECK_DB_PERF`
- **Workspace / Organization**: `REQ_GET_WORKSPACES`, `REQ_CREATE_WORKSPACE`, `REQ_SWITCH_WORKSPACE`, `REQ_CLONE_WORKSPACE`, `REQ_CREATE_ORGANIZATION`, `REQ_JOIN_ORGANIZATION`
- **Agents / Question Sets**: `REQ_CREATE_AGENT`, `REQ_UPDATE_AGENT`, `REQ_DELETE_AGENT`, `REQ_CREATE_QUESTION_SET`, `REQ_UPDATE_QUESTION_SET`, `REQ_UPDATE_QUESTION_SET_AGENTS`, `REQ_IMPORT_QUESTION_SET`, `REQ_EXPORT_QUESTION_SET`
- **Auth / Identity**: `AUTH`, `REQ_WS_LOGIN`, `REQ_WS_REGISTER`, `REQ_WS_BOOTSTRAP_ADMIN`, `REQ_CHECK_ADMIN_EXISTS`, `REQ_CHECK_MANAGER_STATUS`, `REQ_GET_ME`, `REQ_ACCEPT_TERMS`, `REQ_WEBAUTHN_REGISTER_BEGIN`, `REQ_WEBAUTHN_REGISTER_FINISH`, `REQ_WEBAUTHN_LOGIN_BEGIN`, `REQ_WEBAUTHN_LOGIN_FINISH`, `REQ_WEBAUTHN_DELETE_KEY`, `REQ_DEV_GET_MANAGERS`, `REQ_DEV_LOGIN`
- **Admin**: `REQ_ADMIN_GET_USERS`, `REQ_ADMIN_GET_ORGANIZATIONS`, `REQ_ADMIN_GET_USER_PROFILE`, `REQ_ADMIN_GET_ORG_PROFILE`, `REQ_ADMIN_CREATE_USER`, `REQ_ADMIN_CREATE_ORG`, `REQ_ADMIN_UPDATE_USER`, `REQ_ADMIN_DELETE_USER`, `REQ_ADMIN_UPDATE_ORG`, `REQ_ADMIN_DELETE_ORG`, `REQ_ADMIN_GENERATE_INVITE`, `REQ_ADMIN_REMOVE_USER_FROM_ORG`, `REQ_ADMIN_GET_LOGIN_LOGS`
- **Manager**: `REQ_MANAGER_GET_WORKSPACES`, `REQ_MANAGER_GET_AGENTS`, `REQ_MANAGER_GET_RUNS`, `REQ_MANAGER_GET_USERS`, `REQ_MANAGER_CREATE_USER`, `REQ_MANAGER_UPDATE_USER`, `REQ_MANAGER_TOGGLE_USER_SUSPENSION`, `REQ_MANAGER_IMPERSONATE_USER`, `REQ_MANAGER_GET_STATS`, `REQ_MANAGER_GENERATE_INVITE`

Current outbound data responses (`DATA_*`):

- `DATA_RESPONSE`, `DATA_STATE`, `DATA_RUN_DETAILS`, `DATA_RUN_LITE`, `DATA_RESULT_DETAILS`, `DATA_RETRY_STATUS`, `DATA_EVALUATION`, `DATA_SPY_PAYLOAD`, `DATA_WORKSPACE_STATS`, `DATA_WORKSPACE_RUNS`
- `DATA_WORKSPACES`, `DATA_MANAGER_STATUS`, `DATA_ME`, `DATA_CHECK_ADMIN_EXISTS`, `DATA_WS_LOGIN_RESULT`
- `DATA_MANAGER_STATS`, `DATA_MANAGER_USERS`, `DATA_MANAGER_WORKSPACES`, `DATA_MANAGER_AGENTS`, `DATA_MANAGER_RUNS`
- `DATA_ORG_STATS`, `DATA_GLOBAL_STATS`
- `DATA_ADMIN_USERS`, `DATA_ADMIN_ORGANIZATIONS`, `DATA_ADMIN_USER_PROFILE`, `DATA_ADMIN_ORG_PROFILE`, `DATA_ADMIN_LOGIN_LOGS`
- `DATA_DEV_MANAGERS`, `DATA_DEV_LOGIN_RESULT`
- `DATA_WEBAUTHN_REGISTER_OPTIONS`, `DATA_WEBAUTHN_LOGIN_OPTIONS`

Current outbound events (`EVT_*`):

- `EVT_RUN_INIT`, `EVT_RUN_STARTED`, `EVT_RUN_COMPLETED`, `EVT_RUN_CANCELLED`, `EVT_RUN_FINISHED`
- `EVT_TASK_QUEUED`, `EVT_TASK_STARTED`, `EVT_TASK_PROGRESS`, `EVT_TASK_COMPLETED`
- `EVT_DATA_CHANGED`, `EVT_MAINTENANCE_STARTED`, `EVT_FORCE_LOGOUT`, `EVT_ONLINE_STATUS`, `EVT_ERROR`

Compatibility note:

- Constants kept for legacy compatibility but not routed by the main dispatcher include `REQ_CHANGE_PASSWORD`, `CMD_ADMIN_PROFILE`, `CMD_ADMIN_GET_USERS`, `CMD_CHECK_ADMIN_EXISTS`, `CMD_SEED_HISTORY`, and `CMD_SYNC_STATE`.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | `host=localhost...` | Postgres connection |
| `JWT_SECRET` | `dev-secret...` | JWT signing secret |
| `PORT` | `8080` | API port |
| `FIREBASE_CREDENTIALS` | Path to JSON | Firebase Service Account file |

## Development

### Run Tests

```bash
# Backend Tests
cd server_go
go test ./... -v

# Frontend Tests
cd frontend
npm run test
```

### Run Without Docker

```bash
# Terminal 1: Start Postgres
docker run -d -p 5432:5432 -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=benchmarking postgres:15

# Terminal 2: Start Go API
cd server_go
export DATABASE_URL="host=localhost user=postgres password=postgres dbname=benchmarking port=5432 sslmode=disable"
go run .
```

## Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Frontend  │────▶│   Go API    │────▶│  PostgreSQL │
└─────────────┘     └──────┬──────┘     └─────────────┘
                           │
              ┌────────────┴────────────┐
              ▼                         ▼
       ┌─────────────┐           ┌─────────────┐
       │ MCP Servers │           │  OpenAI API │
       └─────────────┘           └─────────────┘
```

## License

Licensed under the Apache License 2.0. See `LICENSE` for details.
