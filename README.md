# Benchmarking Platform

A production-grade, multi-tenant benchmarking platform for AI agents.

## Quick Start

### Using Docker Compose (Recommended)

```bash
# Build and start all services
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

## API Architecture

This platform uses a **WebSocket-first architecture**. All real-time operations (agents, question sets, runs, evaluations, stats) are handled via WebSocket messages.

### REST Endpoints (Minimal)

Only essential auth endpoints use REST:

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check |
| POST | `/auth/register` | User registration |
| POST | `/auth/login` | User login |
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
| **Auth** | `auth.login`, `auth.register`, `auth.logout` |
| **Agents** | `agents.list`, `agents.create`, `agents.update`, `agents.delete`, `agents.reorder` |
| **Question Sets** | `questionSets.list`, `questionSets.get`, `questionSets.create`, `questionSets.update`, `questionSets.delete`, `questionSets.import` |
| **Runs** | `runs.start`, `runs.list`, `runs.get`, `runs.cancel`, `runs.rerun` |
| **Evaluations** | `evaluations.submit`, `evaluations.list`, `evaluations.runOpenAI` |
| **Stats** | `stats.workspace`, `stats.organization`, `stats.global` |
| **Admin** | `admin.users.*`, `admin.organizations.*` |
| **Manager** | `manager.users.*`, `manager.workspaces.*` |
| **Workspace** | `workspaces.list`, `workspaces.create`, `workspaces.switch` |

Real-time events are pushed to connected clients automatically (run progress, task completion, etc.).

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | `host=localhost...` | Postgres connection |
| `PYTHON_RUNNER_URL` | `http://localhost:3003` | Python runner URL |
| `JWT_SECRET` | `dev-secret...` | JWT signing secret |
| `PORT` | `8080` | API port |

## Development

### Run Tests

```bash
cd server_go
go test ./... -v
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
