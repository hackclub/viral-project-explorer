# Viral Project Explorer

A service that exports Hack Club YSWS project and mention data as a SQLite database.

## API Documentation

### Base URL

```
http://localhost:8080
```

### Authentication

All endpoints require API key authentication. Provide the key via one of these methods:

| Method | Header | Example |
|--------|--------|---------|
| X-API-Key header | `X-API-Key: <key>` | `curl -H "X-API-Key: abc123" ...` |
| Bearer token | `Authorization: Bearer <key>` | `curl -H "Authorization: Bearer abc123" ...` |

If `API_KEY` is not set in the environment, a random key is generated on startup and printed to the console.

### Endpoints

#### `GET /db`

Downloads a SQLite database containing YSWS project and mention data.

**Request:**
```bash
curl -H "X-API-Key: YOUR_API_KEY" http://localhost:8080/db -o database.db
```

**Response:**
- **200 OK**: Returns SQLite database file (`application/octet-stream`)
- **401 Unauthorized**: Missing or invalid API key

**Response Headers:**
```
Content-Type: application/octet-stream
Content-Disposition: attachment; filename="database.db"
```

---

## SQLite Schema

The downloaded database contains two tables with an index for efficient joins.

### `approved_projects`

Contains YSWS approved project information.

| Column | Type | Description |
|--------|------|-------------|
| `record_id` | TEXT | **Primary key.** Airtable record ID |
| `first_name` | TEXT | Project author's first name |
| `git_hub_username` | TEXT | Author's GitHub username |
| `geocoded_country` | TEXT | Country name (geocoded) |
| `hack_clubber_geocoded_country` | TEXT | Alternative geocoded country |
| `geocoded_country_code` | TEXT | ISO country code (e.g., US, IN) |
| `playable_url` | TEXT | Live/playable URL for the project |
| `code_url` | TEXT | Source code URL |

### `ysws_project_mentions`

Contains mentions/references to YSWS projects found across the web.

| Column | Type | Description |
|--------|------|-------------|
| `id` | TEXT | **Primary key.** Mention record ID |
| `ysws_project_mentions_id` | TEXT | Internal mention ID |
| `ysws_project_mention_searches` | TEXT | Search ID reference |
| `ysws_from_ysws_approved_project` | TEXT | Source approved project reference |
| `record_id` | TEXT | Mention's own record ID |
| `ysws_approved_project` | TEXT | **Foreign key** → `approved_projects.record_id` |
| `source` | TEXT | Source platform (YouTube, Reddit, etc.) |
| `link_found_at` | TEXT | URL where mention was found |
| `archive_url` | TEXT | Archive.org URL |
| `url` | TEXT | Direct URL to the mention |
| `headline` | TEXT | Title/headline of the mention |
| `date` | TEXT | Date of the mention |
| `weighted_engagement_points` | REAL | Calculated engagement score |
| `project_url` | TEXT | URL of the project being mentioned |
| `engagement_count` | INTEGER | Raw engagement count |
| `engagement_type` | TEXT | Type of engagement metric |
| `mentions_hack_club` | INTEGER | 1 if mentions Hack Club, 0 otherwise |
| `published_by_hack_club` | INTEGER | 1 if published by Hack Club, 0 otherwise |

### Indexes

| Index | Table | Column |
|-------|-------|--------|
| `idx_mentions_record_id` | `ysws_project_mentions` | `record_id` |

### Joining Tables

To join projects with their mentions:

```sql
SELECT 
    ap.first_name,
    ap.git_hub_username,
    pm.source,
    pm.headline,
    pm.engagement_count
FROM approved_projects ap
JOIN ysws_project_mentions pm 
    ON ap.record_id = pm.ysws_approved_project
ORDER BY pm.engagement_count DESC;
```

### Example Queries

**Top projects by total mentions:**
```sql
SELECT 
    ap.first_name,
    ap.git_hub_username,
    COUNT(pm.id) as mention_count
FROM approved_projects ap
JOIN ysws_project_mentions pm ON ap.record_id = pm.ysws_approved_project
GROUP BY ap.record_id
ORDER BY mention_count DESC
LIMIT 10;
```

**Mentions by source platform:**
```sql
SELECT source, COUNT(*) as count
FROM ysws_project_mentions
GROUP BY source
ORDER BY count DESC;
```

**Projects from a specific country:**
```sql
SELECT first_name, git_hub_username, playable_url
FROM approved_projects
WHERE geocoded_country_code = 'US';
```

---

## Running the Backend

```bash
cd backend

# Set environment variables (or use .env file)
export WAREHOUSE_READONLY_UNIFIED_YSWS_DATABASE_URL="postgres://..."
export API_KEY="your-secret-key"  # Optional, auto-generated if not set

# Install dependencies and run
go mod tidy
go run main.go
```

The server starts on port `8080` by default.

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `WAREHOUSE_READONLY_UNIFIED_YSWS_DATABASE_URL` | Yes | PostgreSQL connection string |
| `API_KEY` | No | API key for authentication (auto-generated if not set) |


