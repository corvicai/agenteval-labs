# WebSocket Envelope Messages Reference

Complete reference of all WebSocket message types used by the Benchmarking Platform. All messages use the same envelope format.

## Envelope Format

```json
{
  "type": "REQ_GET_ME",
  "correlation_id": "uuid-or-client-id",
  "payload": {}
}
```

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Message type identifier (REQ_*, CMD_*, DATA_*, EVT_*) |
| `correlation_id` | string | Client-generated ID to match requests with responses |
| `payload` | object | Request/response data (JSON) |

## Message Type Prefixes

| Prefix | Direction | Description |
|--------|-----------|-------------|
| `REQ_` | Client → Server | Request expecting a `DATA_` response |
| `CMD_` | Client → Server | Command (may trigger events or state changes) |
| `DATA_` | Server → Client | Response to a request, matched by `correlation_id` |
| `EVT_` | Server → Client | Unsolicited event (broadcast) |

---

## Authentication & Identity

### AUTH (Firebase Social Auth)
**Type**: `AUTH`  
**Payload**:
```json
{ "token": "firebase_id_token" }
```
**Response**: `DATA_WS_LOGIN_RESULT` — `{ "success": true, "token": "jwt", "requires_onboarding": bool, "requires_terms": bool, "user": {...}, "workspace": {...} }`

### REQ_WS_LOGIN
**Payload**:
```json
{
  "email": "string",
  "password": "string",
  "organization_id": "uuid (optional)"
}
```
**Response**: `DATA_WS_LOGIN_RESULT`

### REQ_WS_REGISTER
**Payload**:
```json
{
  "name": "string",
  "email": "string",
  "password": "string",
  "invite_code": "string",
  "organization_name": "string (optional)",
  "role": "manager | user"
}
```
**Response**: `DATA_WS_LOGIN_RESULT`

### REQ_WS_BOOTSTRAP_ADMIN
**Payload**:
```json
{
  "name": "string",
  "email": "string",
  "password": "string",
  "organization_name": "string"
}
```
**Response**: `DATA_WS_LOGIN_RESULT`

### REQ_GET_ME
**Payload**: `{}`  
**Response**: `DATA_ME` — `{ "user": {...}, "organization": {...} }`

### REQ_CHECK_ADMIN_EXISTS
**Payload**: `{}`  
**Response**: `DATA_CHECK_ADMIN_EXISTS` — `{ "exists": bool }`

### REQ_CHECK_MANAGER_STATUS
**Payload**: `{}`  
**Response**: `DATA_MANAGER_STATUS` — `{ "is_manager": bool }`

### REQ_ACCEPT_TERMS
**Payload**: `{}`  
**Response**: `DATA_RESPONSE` — `{ "status": "success" }`

### REQ_CHANGE_PASSWORD
**Payload**:
```json
{
  "id": "uuid (optional, for admin resetting others)",
  "old_password": "string (required for self-change)",
  "new_password": "string"
}
```
**Response**: `DATA_RESPONSE`

### WebAuthn

| Type | Response | Notes |
|------|----------|-------|
| `REQ_WEBAUTHN_REGISTER_BEGIN` | `DATA_WEBAUTHN_REGISTER_OPTIONS` | Passkey registration start |
| `REQ_WEBAUTHN_REGISTER_FINISH` | `DATA_RESPONSE` | Passkey registration finish |
| `REQ_WEBAUTHN_LOGIN_BEGIN` | `DATA_WEBAUTHN_LOGIN_OPTIONS` | Passkey login start |
| `REQ_WEBAUTHN_LOGIN_FINISH` | `DATA_WS_LOGIN_RESULT` | Passkey login finish |
| `REQ_WEBAUTHN_DELETE_KEY` | `DATA_RESPONSE` | Delete passkey |

---

## Workspace & Organization

### REQ_GET_WORKSPACES
**Payload**: `{}`  
**Response**: `DATA_WORKSPACES` — Array of workspaces with `agent_count`

### REQ_CREATE_WORKSPACE
**Payload**: `{ "name": "string" }`  
**Response**: `DATA_RESPONSE` — Workspace object

### REQ_SWITCH_WORKSPACE
**Payload**: `{ "workspace_id": "uuid" }`  
**Response**: `DATA_RESPONSE` — `{ "token": "jwt", "workspace": {...} }`

### REQ_CLONE_WORKSPACE
**Payload**: `{ "source_workspace_id": "uuid", "new_name": "string" }`  
**Response**: `DATA_RESPONSE` — New workspace with `agent_count`

### REQ_CREATE_ORGANIZATION
**Payload**: `{ "name": "string" }`  
**Response**: `DATA_RESPONSE`

### REQ_JOIN_ORGANIZATION
**Payload**: `{ "invite_code": "string" }`  
**Response**: `DATA_RESPONSE`

### REQ_GET_WORKSPACE_CLIENTS
**Payload**: `{}`  
**Response**: `DATA_RESPONSE` — Array of clients

---

## Agents & Question Sets

### REQ_CREATE_AGENT
**Payload**:
```json
{
  "workspace_id": "uuid",
  "name": "string",
  "provider_type": "string",
  "config": { ... }
}
```
**Response**: `DATA_RESPONSE` — Agent object  
**Broadcast**: `EVT_DATA_CHANGED` (resource: `agents`, action: `created`)

### REQ_UPDATE_AGENT
**Payload**:
```json
{
  "id": "uuid",
  "name": "string",
  "provider_type": "string",
  "config": { ... },
  "enabled": bool,
  "position": int,
  "max_concurrency": int
}
```
**Response**: `DATA_RESPONSE` — Updated agent  
**Broadcast**: `EVT_DATA_CHANGED` (resource: `agents`, action: `updated`)

### REQ_DELETE_AGENT
**Payload**: `{ "id": "uuid" }`  
**Response**: `DATA_RESPONSE` — `{ "status": "success" }`  
**Broadcast**: `EVT_DATA_CHANGED` (resource: `agents`, action: `deleted`)

### REQ_CREATE_QUESTION_SET
**Payload**: `{ "workspace_id": "uuid", "name": "string", "data": {...} }`  
**Response**: `DATA_RESPONSE` — Question set object  
**Broadcast**: `EVT_DATA_CHANGED` (resource: `question_sets`, action: `created`)

### REQ_UPDATE_QUESTION_SET
**Payload**: `{ "id": "uuid", "name": "string", "data": {...} }`  
**Response**: `DATA_RESPONSE`  
**Broadcast**: `EVT_DATA_CHANGED` (resource: `question_sets`, action: `updated`)

### REQ_UPDATE_QUESTION_SET_AGENTS
**Payload**:
```json
{
  "question_set_id": "uuid",
  "agents": [
    {
      "agent_id": "uuid",
      "config": { ... },
      "enabled": bool,
      "position": int
    }
  ]
}
```
**Response**: `DATA_RESPONSE` — Updated question set with agents  
**Broadcast**: `EVT_DATA_CHANGED` (resource: `question_sets`, action: `updated`)

### REQ_GET_QUESTION_SET_AGENT_ENVELOPE
**Payload**: `{ "question_set_id": "uuid" }`  
**Response**: `DATA_RESPONSE` — `{ "question_set_id": "uuid", "selected_agents": [...], "available_agents": [...] }`

### REQ_IMPORT_QUESTION_SET
**Payload**: `{ "client_id": "uuid", "name": "string", "data": {...} }`  
**Response**: `DATA_RESPONSE` — Created question set  
**Broadcast**: `EVT_DATA_CHANGED` (resource: `question_sets`, action: `created`)

### REQ_EXPORT_QUESTION_SET
**Payload**: `{ "id": "uuid" }`  
**Response**: `DATA_RESPONSE` — Question set data object

---

## Benchmark Runs

### CMD_START_RUN
**Payload**:
```json
{
  "question_set_id": "uuid",
  "agent_ids": ["uuid", ...]
}
```
`agent_ids` is optional; defaults to all enabled agents for the question set.  
**Response**: Run object (via `DATA_RESPONSE`)

### CMD_CANCEL_RUN
**Payload**: `{ "run_id": "uuid" }`  
**Response**: `{ "status": "cancelled" }`

### CMD_RERUN_TASK
**Payload**:
```json
{
  "run_id": "uuid",
  "agent_id": "uuid",
  "question_id": "string",
  "question_set_id": "uuid (optional)",
  "result_id": "uuid (optional)",
  "original_question": "string (optional)",
  "expected_answer": "string (optional)"
}
```
**Response**: `{ "status": "queued", "retry_id": "string" }`

### CMD_RUN_EVALUATORS
**Payload**: Run evaluators for a run (see handler for details)  
**Response**: `DATA_RESPONSE`

### REQ_SYNC_STATE
**Payload**: `{}`  
**Response**: `DATA_STATE` — `{ "agents": [...], "question_sets": [...], "recent_runs": [...] }`

### REQ_GET_RUN_DETAILS
**Payload**: `{ "run_id": "uuid" }`  
**Response**: `DATA_RUN_DETAILS` — Run with QuestionSet, Results (with Evaluations), Agents map

### REQ_GET_RUN_LITE
**Payload**: `{ "run_id": "uuid" }`  
**Response**: `DATA_RUN_LITE` — Run metadata and result headers (no full answer text; SHA256 hash instead)

### REQ_GET_LATEST_RUN_BY_QS
**Payload**: `{ "question_set_id": "uuid" }`  
**Response**: `DATA_RUN_LITE` or null if no completed run

### REQ_GET_RESULT_DETAILS
**Payload**: `{ "result_ids": ["uuid", ...] }`  
**Response**: `DATA_RESULT_DETAILS` — Full RunResult objects including Evaluations

### REQ_GET_RETRY_STATUS
**Payload**: `{ "retry_ids": ["string", ...] }`  
**Response**: `DATA_RETRY_STATUS` — Status items for each retry

### REQ_GET_WORKSPACE_RUNS
**Payload**: `{}`  
**Response**: `DATA_WORKSPACE_RUNS` — Runs for current workspace

### REQ_GET_SPY_PAYLOAD
**Payload**: Spy payload for debugging (see handler)  
**Response**: `DATA_SPY_PAYLOAD`

### REQ_CREATE_EVALUATION
**Payload**: Create user evaluation for a result  
**Response**: `DATA_EVALUATION`

### REQ_DELETE_RUN
**Payload**: `{ "run_id": "uuid" }`  
**Response**: `DATA_RESPONSE`

### REQ_DELETE_ALL_RUNS
**Payload**: `{}`  
**Response**: `DATA_RESPONSE` — Deletes all runs for workspace

---

## Stats

### REQ_GET_WORKSPACE_STATS
**Payload**: `{ "workspace_id": "uuid", "force": bool }`  
**Response**: `DATA_WORKSPACE_STATS` — Aggregated stats (cache TTL: 5 min unless `force=true`)

### REQ_GET_ORG_STATS
**Payload**: `{ "force": bool }` (optional)  
**Response**: `DATA_ORG_STATS` — Aggregated stats (cache TTL: 15 min)

### REQ_GET_GLOBAL_STATS
**Payload**: `{ "force": bool }` (optional)  
**Response**: `DATA_GLOBAL_STATS` — Aggregated stats (cache TTL: 30 min, admin only)

### REQ_GET_MANAGER_STATS
**Payload**: `{}`  
**Response**: `DATA_MANAGER_STATS` — `{ "user_count", "workspace_count", "agent_count", "run_count" }`

### REQ_GET_MANAGER_USERS
**Payload**: `{}`  
**Response**: `DATA_MANAGER_USERS` — Array of UserResponse

---

## Admin

| Type | Response | Notes |
|------|----------|-------|
| `REQ_ADMIN_GET_USERS` | `DATA_ADMIN_USERS` | Optional `time_range`: "24h", "3d", "1w" |
| `REQ_ADMIN_GET_ORGANIZATIONS` | `DATA_ADMIN_ORGANIZATIONS` | Optional `time_range` |
| `REQ_ADMIN_GET_USER_PROFILE` | `DATA_ADMIN_USER_PROFILE` | Payload: `{ "id": "uuid" }` |
| `REQ_ADMIN_GET_ORG_PROFILE` | `DATA_ADMIN_ORG_PROFILE` | Payload: `{ "id": "uuid" }` |
| `REQ_ADMIN_CREATE_USER` | `DATA_RESPONSE` | name, email, password, is_admin, organization_id, role, workspace_name |
| `REQ_ADMIN_CREATE_ORG` | `DATA_RESPONSE` | name, manager_id |
| `REQ_ADMIN_UPDATE_USER` | `DATA_RESPONSE` | id, name, email, is_admin, is_suspended, organization_id, role |
| `REQ_ADMIN_DELETE_USER` | `DATA_RESPONSE` | id, mode: "hard" \| "ghost" |
| `REQ_ADMIN_UPDATE_ORG` | `DATA_RESPONSE` | id, name, manager_id, manager_ids, is_suspended |
| `REQ_ADMIN_DELETE_ORG` | `DATA_RESPONSE` | id |
| `REQ_ADMIN_GENERATE_INVITE` | `DATA_RESPONSE` | target_org_id, is_new_org, max_uses |
| `REQ_ADMIN_REMOVE_USER_FROM_ORG` | `DATA_RESPONSE` | user_id, organization_id |
| `REQ_ADMIN_GET_LOGIN_LOGS` | `DATA_ADMIN_LOGIN_LOGS` | Optional `limit` (default 100, max 500) |

### CMD_ADMIN_FORCE_LOGOUT
**Payload**: `{ "user_id": "uuid" }` (optional; empty = all users)  
**Broadcast**: `EVT_FORCE_LOGOUT`

### CMD_ADMIN_START_MAINTENANCE
**Payload**: `{}`  
**Broadcast**: `EVT_MAINTENANCE_STARTED` to all users

---

## Manager

| Type | Response | Notes |
|------|----------|-------|
| `REQ_MANAGER_GET_WORKSPACES` | `DATA_MANAGER_WORKSPACES` | Workspaces with user_name, agent_count, run_count |
| `REQ_MANAGER_GET_AGENTS` | `DATA_MANAGER_AGENTS` | Agents with workspace_name, user_name |
| `REQ_MANAGER_GET_RUNS` | `DATA_MANAGER_RUNS` | Last 100 runs with metadata |
| `REQ_MANAGER_GET_USERS` | `DATA_MANAGER_USERS` | Users with workspace_count, invited_by_name |
| `REQ_MANAGER_CREATE_USER` | `DATA_RESPONSE` | name, email, password |
| `REQ_MANAGER_UPDATE_USER` | `DATA_RESPONSE` | id, name, email |
| `REQ_MANAGER_TOGGLE_USER_SUSPENSION` | `DATA_RESPONSE` | id |
| `REQ_MANAGER_IMPERSONATE_USER` | `DATA_RESPONSE` | user_id — returns JWT for target user |
| `REQ_MANAGER_GET_STATS` | `DATA_MANAGER_STATS` | user_count, workspace_count, agent_count, run_count |
| `REQ_MANAGER_GENERATE_INVITE` | `DATA_RESPONSE` | Optional max_uses |

---

## Dev-Only (Disabled in Production)

| Type | Response | Notes |
|------|----------|-------|
| `REQ_DEV_GET_MANAGERS` | `DATA_DEV_MANAGERS` | Array of manager info |
| `REQ_DEV_LOGIN` | `DATA_DEV_LOGIN_RESULT` | Payload: `{ "user_id": "uuid" }` — passwordless login |
| `CMD_SEED_HISTORICAL_RUN` | `DATA_RESPONSE` | Backdate a run with results |
| `CMD_ADMIN_RECALCULATE_STATS` | `DATA_RESPONSE` | Clears stats cache |
| `REQ_CHECK_DB_PERF` | `DATA_RESPONSE` | Simple DB ping test |

---

## Server Events (EVT_*)

| Type | When | Payload |
|------|------|---------|
| `EVT_RUN_INIT` | Run initialized | run_id, question_set_id, etc. |
| `EVT_RUN_STARTED` | Run started | run_id, status |
| `EVT_RUN_COMPLETED` | Run completed | run_id, status |
| `EVT_RUN_CANCELLED` | Run cancelled | run_id |
| `EVT_RUN_FINISHED` | All tasks done | run_id, status, total_tasks, completed |
| `EVT_TASK_QUEUED` | Task queued | run_id, agent_id, question_id |
| `EVT_TASK_STARTED` | Task started | run_id, agent_id, question_id |
| `EVT_TASK_PROGRESS` | Task progress (long runs) | run_id, agent_id, question_id, elapsed_ms, message |
| `EVT_TASK_COMPLETED` | Task completed | run_id, agent_id, question_id, success, answer, duration_ms |
| `EVT_DATA_CHANGED` | Resource changed | resource, action, data |
| `EVT_MAINTENANCE_STARTED` | Maintenance mode | — |
| `EVT_FORCE_LOGOUT` | Admin forced logout | — |
| `EVT_ONLINE_STATUS` | User online/offline | total, user_ids |
| `EVT_ERROR` | Error response | error, details (dev only) |

---

## Legacy / Compatibility

These constants exist for backward compatibility but are not routed by the main dispatcher:

- `CMD_ADMIN_PROFILE`
- `CMD_ADMIN_GET_USERS`
- `CMD_CHECK_ADMIN_EXISTS`
- `CMD_SEED_HISTORY`
- `CMD_SYNC_STATE`
