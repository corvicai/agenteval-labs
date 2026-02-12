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
- **Python Runner** on port `3003`
- **Go API** on port `8080`

### Verify Services

```bash
# Check Go API health
curl http://localhost:8080/health

# Check Python Runner health
curl http://localhost:3003/health
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

All operations are handled via WebSocket message types:

| Category | Message Types |
|----------|---------------|
| **Auth** | `AUTH` (Firebase), `REQ_WEBAUTHN_*`, `REQ_WS_LOGIN` (Dev), `REQ_WS_BOOTSTRAP_ADMIN` |
| **Commands** | `CMD_START_RUN`, `CMD_CANCEL_RUN`, `CMD_RUN_EVALUATORS`, `CMD_RERUN_TASK` |
| **Workspaces** | `REQ_GET_WORKSPACES`, `REQ_CREATE_WORKSPACE`, `REQ_SWITCH_WORKSPACE`, `REQ_CLONE_WORKSPACE` |
| **Organizations** | `REQ_CREATE_ORGANIZATION`, `REQ_JOIN_ORGANIZATION`, `REQ_GET_ORG_STATS` |
| **Agents** | `REQ_CREATE_AGENT`, `REQ_UPDATE_AGENT`, `REQ_DELETE_AGENT` |
| **Question Sets** | `REQ_CREATE_QUESTION_SET`, `REQ_UPDATE_QUESTION_SET`, `REQ_IMPORT_QUESTION_SET` |
| **Runs** | `REQ_GET_RUN_DETAILS`, `REQ_GET_RUN_LITE`, `REQ_GET_LATEST_RUN_BY_QS`, `REQ_GET_RESULT_DETAILS` |
| **Admin** | `REQ_ADMIN_GET_USERS`, `REQ_ADMIN_CREATE_USER`, `REQ_ADMIN_GENERATE_INVITE` |

Real-time events are pushed to connected clients automatically (run progress, task completion, etc.).

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | `host=localhost...` | Postgres connection |
| `PYTHON_RUNNER_URL` | `http://localhost:3003` | Python runner URL |
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

# Terminal 2: Start Python Runner
cd server_python
pip install -r requirements.txt
python server.py

# Terminal 3: Start Go API
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
                           ▼
                    ┌─────────────┐
                    │   Python    │
                    │   Runner    │
                    └──────┬──────┘
                           │
              ┌────────────┴────────────┐
              ▼                         ▼
       ┌─────────────┐           ┌─────────────┐
       │ MCP Servers │           │  OpenAI API │
       └─────────────┘           └─────────────┘
```

## License

Internal use only.

# TMP

Nothing to see here, move along.
