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

## API Endpoints

### Agents
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/workspaces/:ws_id/agents` | List agents |
| POST | `/workspaces/:ws_id/agents` | Create agent |
| POST | `/workspaces/:ws_id/agents/reorder` | Reorder agents |
| GET | `/agents/:id` | Get agent |
| PUT | `/agents/:id` | Update agent |
| DELETE | `/agents/:id` | Delete agent |
| GET | `/agents/:id/spy` | Preview payload (secrets redacted) |

### Question Sets
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/clients/:client_id/question-sets` | List question sets |
| POST | `/clients/:client_id/question-sets/import` | Import JSON |
| GET | `/question-sets/:id` | Get question set |
| GET | `/question-sets/:id/export` | Export JSON |
| DELETE | `/question-sets/:id` | Delete |

### Runs
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/workspaces/:ws_id/runs` | Start run |
| GET | `/runs/:run_id` | Get status |
| POST | `/runs/:run_id/rerun` | Rerun task |
| POST | `/runs/:run_id/evaluate` | Trigger evaluators |
| POST | `/runs/:run_id/cancel` | Cancel run |
| GET | `/workspaces/:ws_id/history` | Answer history |

### Evaluations
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/evaluations` | Submit rating |
| GET | `/run-results/:id/evaluations` | List evaluations |
| DELETE | `/evaluations/:id` | Delete |

### WebSocket
| Endpoint | Description |
|----------|-------------|
| `GET /ws?workspace_id=<uuid>` | Real-time progress |

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
