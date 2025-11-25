# AI Agent Development Guide

This document provides instructions for AI agents to work with the development environment. **The dev environment runs in Docker with live reload** — you do NOT need to restart servers when making code changes.

## Quick Reference

| Action | Command |
|--------|---------|
| Start dev environment | `docker compose up -d` |
| Check if running | `docker compose ps` |
| View all logs | `docker compose logs -f` |
| View backend logs | `docker compose logs -f backend` |
| View frontend logs | `docker compose logs -f frontend` |
| Stop environment | `docker compose down` |
| Rebuild after Dockerfile changes | `docker compose up -d --build` |

## Development Environment Overview

The project uses Docker Compose with two services:

| Service | Port | Technology | Live Reload |
|---------|------|------------|-------------|
| `backend` | 8080 | Go + Air | ✅ Rebuilds on `.go` file changes |
| `frontend` | 5173 | SvelteKit + Vite | ✅ HMR on file changes |

### Live Reload Behavior

- **Backend**: Uses [Air](https://github.com/air-verse/air) to watch for Go file changes. When you edit any `.go` file, Air automatically rebuilds and restarts the server (typically 1-2 seconds).
- **Frontend**: Uses Vite's built-in Hot Module Replacement (HMR). Changes to Svelte components, CSS, and TypeScript are reflected instantly in the browser without full page reload.

**⚠️ IMPORTANT: Do NOT kill or restart the servers when making code changes. Live reload handles this automatically.**

---

## Starting the Development Environment

### Prerequisites

1. Docker and Docker Compose must be installed
2. Create a `.env` file in the project root with required environment variables:

```bash
# Required for backend
WAREHOUSE_READONLY_UNIFIED_YSWS_DATABASE_URL=postgres://...

# Optional - auto-generated if not set
API_KEY=your-api-key
```

### First-Time Setup

```bash
# From project root
docker compose up -d --build
```

### Subsequent Starts

```bash
docker compose up -d
```

---

## Checking Environment Status

### Is the environment running?

```bash
docker compose ps
```

Expected output when running:
```
NAME                              IMAGE                                  COMMAND                  SERVICE    PORTS                    STATUS
viral-project-explorer-backend    viral-project-explorer-backend         "air -c .air.toml"       backend    0.0.0.0:8080->8080/tcp   Up X minutes (healthy)
viral-project-explorer-frontend   viral-project-explorer-frontend        "npm run dev -- --h…"   frontend   0.0.0.0:5173->5173/tcp   Up X minutes (healthy)
```

If services show as "Exited" or aren't listed, the environment is not running.

### Check service health

```bash
# Backend health (should return HTTP response or connection info)
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/db

# Frontend health (should return 200)
curl -s -o /dev/null -w "%{http_code}" http://localhost:5173
```

---

## Viewing Logs

### Follow all logs (real-time)

```bash
docker compose logs -f
```

### Follow specific service logs

```bash
# Backend only
docker compose logs -f backend

# Frontend only  
docker compose logs -f frontend
```

### View recent logs (last 100 lines)

```bash
docker compose logs --tail=100
```

### View logs without following

```bash
docker compose logs
```

---

## Making Code Changes

### Backend (Go)

1. Edit any `.go` file in `backend/`
2. Air automatically detects the change, rebuilds, and restarts
3. Watch the backend logs to confirm rebuild:
   ```bash
   docker compose logs -f backend
   ```
4. You'll see output like:
   ```
   backend  | watching .
   backend  | building...
   backend  | running...
   backend  | Server starting on port :8080
   ```

### Frontend (Svelte/TypeScript)

1. Edit any file in `frontend/src/`
2. Vite HMR instantly updates the browser
3. No action needed — changes appear automatically
4. For debugging, check frontend logs:
   ```bash
   docker compose logs -f frontend
   ```

---

## Stopping the Environment

### Graceful stop (preserves containers)

```bash
docker compose stop
```

### Full stop (removes containers)

```bash
docker compose down
```

### Stop and remove volumes (clean slate)

```bash
docker compose down -v
```

---

## Troubleshooting

### Service won't start

Check logs for errors:
```bash
docker compose logs backend
docker compose logs frontend
```

Common issues:
- Missing `.env` file or required environment variables
- Port already in use (8080 or 5173)

### Live reload not working

**Backend:**
1. Check Air is running: `docker compose logs backend | grep -i air`
2. Ensure file is in `backend/` directory
3. Check file extension is `.go`

**Frontend:**
1. Check Vite is running: `docker compose logs frontend | grep -i vite`
2. Try hard refresh in browser (Ctrl+Shift+R)
3. Check for TypeScript/syntax errors in logs

### Rebuild from scratch

If things get into a bad state:
```bash
docker compose down -v
docker compose up -d --build
```

### Port conflicts

If ports 8080 or 5173 are in use:
```bash
# Find what's using the port
lsof -i :8080
lsof -i :5173

# Or change ports in docker-compose.yml
```

---

## Architecture Reference

```
viral-project-explorer/
├── backend/                    # Go API server
│   ├── main.go                # Main application entry
│   ├── go.mod                 # Go dependencies
│   ├── Dockerfile.dev         # Development container
│   └── .air.toml              # Air live reload config
├── frontend/                   # SvelteKit app
│   ├── src/
│   │   ├── routes/            # SvelteKit routes
│   │   └── app.css            # Global styles
│   ├── package.json           # Node dependencies
│   ├── Dockerfile.dev         # Development container
│   └── vite.config.js         # Vite configuration
├── docker-compose.yml         # Development orchestration
├── .env                       # Environment variables (create this)
└── AGENTS.md                  # This file
```

### Backend API

- **Port**: 8080
- **Endpoint**: `GET /db` — Downloads SQLite database
- **Auth**: Requires `X-API-Key` header or `Authorization: Bearer <key>`

### Frontend

- **Port**: 5173
- **Framework**: SvelteKit with Vite
- **Dev URL**: http://localhost:5173

---

## For AI Agents: Key Guidelines

1. **Always check if environment is running first**: `docker compose ps`
2. **Never kill the servers** — live reload handles code changes automatically
3. **Use logs to verify changes took effect**: `docker compose logs -f <service>`
4. **If you need a fresh start**, use `docker compose down && docker compose up -d --build`
5. **Multiple agents can work simultaneously** since live reload eliminates the need to restart services
6. **Environment variables** are in `.env` at project root — backend reads from there automatically

