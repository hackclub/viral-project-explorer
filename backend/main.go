package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

var (
	apiKey string
	pgDB   *sql.DB

	// Cache for the generated SQLite database
	cacheMutex     sync.RWMutex
	cachedDBPath   string
	cacheCreatedAt time.Time
	cacheTTL       = 5 * time.Minute
)

// Custom logger with timestamps
type Logger struct {
	prefix string
}

func (l *Logger) Info(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Printf("[INFO]  %s%s", l.prefix, msg)
}

func (l *Logger) Error(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Printf("[ERROR] %s%s", l.prefix, msg)
}

func (l *Logger) Warn(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Printf("[WARN]  %s%s", l.prefix, msg)
}

func (l *Logger) Debug(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Printf("[DEBUG] %s%s", l.prefix, msg)
}

var appLog = &Logger{}

func generateAPIKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func generateRequestID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func main() {
	// Configure log format with timestamps
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	appLog.Info("Starting Viral Project Explorer backend...")

	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		if err2 := godotenv.Load("../.env"); err2 != nil {
			appLog.Warn(".env file not found or couldn't be loaded: %v", err)
		}
	} else {
		appLog.Info("Loaded .env file")
	}

	// Get API key from environment variable, or generate one if not set
	apiKey = os.Getenv("API_KEY")
	if apiKey == "" {
		var err error
		apiKey, err = generateAPIKey()
		if err != nil {
			appLog.Error("Failed to generate API key: %v", err)
			os.Exit(1)
		}
		fmt.Println("")
		fmt.Println("=" + strings.Repeat("=", 70) + "=")
		fmt.Println("⚠️  API_KEY not set in environment")
		fmt.Println("🔑 Generated API key (use this for authentication):")
		fmt.Println("")
		fmt.Printf("   %s\n", apiKey)
		fmt.Println("")
		fmt.Println("   Or include it in your requests:")
		fmt.Printf("   curl -H \"X-API-Key: %s\" http://localhost:8080/db\n", apiKey)
		fmt.Println("=" + strings.Repeat("=", 70) + "=")
		fmt.Println("")
	} else {
		appLog.Info("Using API key from environment")
	}

	// Connect to PostgreSQL
	dbURL := os.Getenv("WAREHOUSE_READONLY_UNIFIED_YSWS_DATABASE_URL")
	if dbURL == "" {
		appLog.Error("WAREHOUSE_READONLY_UNIFIED_YSWS_DATABASE_URL environment variable is required")
		os.Exit(1)
	}

	appLog.Info("Connecting to PostgreSQL...")
	var err error
	pgDB, err = sql.Open("postgres", dbURL)
	if err != nil {
		appLog.Error("Failed to open PostgreSQL connection: %v", err)
		os.Exit(1)
	}
	defer pgDB.Close()

	// Configure connection pool
	pgDB.SetMaxOpenConns(10)
	pgDB.SetMaxIdleConns(5)
	pgDB.SetConnMaxLifetime(5 * time.Minute)

	if err := pgDB.Ping(); err != nil {
		appLog.Error("Failed to ping PostgreSQL database: %v", err)
		os.Exit(1)
	}
	appLog.Info("✓ Connected to PostgreSQL database")

	// Create a mux to handle all routes with authentication
	mux := http.NewServeMux()
	mux.HandleFunc("/db", dbHandler)

	// Chain middleware: logging -> cors -> auth -> handler
	handler := loggingMiddleware(corsMiddleware(authMiddleware(mux)))

	port := ":8080"
	appLog.Info("Server starting on port %s", port)
	appLog.Info("API key authentication is enabled")
	appLog.Info("Endpoint: GET /db - Download SQLite database")

	if err := http.ListenAndServe(port, handler); err != nil {
		appLog.Error("Server failed: %v", err)
		os.Exit(1)
	}
}

// corsMiddleware adds CORS headers to allow cross-origin requests
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow requests from any origin (for development)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")

		// Handle preflight OPTIONS request
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs all incoming requests with timing
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		requestID := generateRequestID()

		// Create a response wrapper to capture status code
		wrapped := &responseWrapper{ResponseWriter: w, statusCode: http.StatusOK}

		// Get client IP
		clientIP := r.RemoteAddr
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			clientIP = strings.Split(forwarded, ",")[0]
		}

		// Log request start
		reqLog := &Logger{prefix: fmt.Sprintf("[%s] ", requestID)}
		reqLog.Info("→ %s %s from %s", r.Method, r.URL.Path, clientIP)

		// Process request
		next.ServeHTTP(wrapped, r)

		// Log request completion
		duration := time.Since(start)
		if wrapped.statusCode >= 400 {
			reqLog.Warn("← %d %s (%s)", wrapped.statusCode, http.StatusText(wrapped.statusCode), duration)
		} else {
			reqLog.Info("← %d %s (%s)", wrapped.statusCode, http.StatusText(wrapped.statusCode), duration)
		}
	})
}

// responseWrapper captures the status code for logging
type responseWrapper struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWrapper) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		apiKeyHeader := r.Header.Get("X-API-Key")

		var providedKey string
		var authMethod string

		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
				providedKey = parts[1]
				authMethod = "Bearer"
			} else {
				providedKey = authHeader
				authMethod = "Authorization"
			}
		} else if apiKeyHeader != "" {
			providedKey = apiKeyHeader
			authMethod = "X-API-Key"
		}

		if providedKey == "" {
			appLog.Warn("Auth failed: no API key provided")
			w.Header().Set("WWW-Authenticate", `Bearer realm="API"`)
			http.Error(w, "Unauthorized: API key is required", http.StatusUnauthorized)
			return
		}

		if providedKey != apiKey {
			appLog.Warn("Auth failed: invalid API key (method: %s)", authMethod)
			w.Header().Set("WWW-Authenticate", `Bearer realm="API"`)
			http.Error(w, "Unauthorized: API key is required", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func dbHandler(w http.ResponseWriter, r *http.Request) {
	requestStart := time.Now()

	// Check if we have a valid cached database
	dbPath, fromCache := getCachedDB()
	if fromCache {
		appLog.Info("Serving cached database (age: %s)", time.Since(cacheCreatedAt).Round(time.Second))
		serveCachedDB(w, dbPath, requestStart)
		return
	}

	// Generate a new database
	newPath, err := generateDB()
	if err != nil {
		appLog.Error("Failed to generate database: %v", err)
		http.Error(w, fmt.Sprintf("Failed to generate database: %v", err), http.StatusInternalServerError)
		return
	}

	appLog.Info("Generated fresh database, caching for %s", cacheTTL)
	serveCachedDB(w, newPath, requestStart)
}

// getCachedDB checks if we have a valid cached database and returns its path
// Returns (path, true) if cache is valid, ("", false) if cache needs refresh
func getCachedDB() (string, bool) {
	cacheMutex.RLock()
	defer cacheMutex.RUnlock()

	// Check if cache exists and is still valid
	if cachedDBPath == "" || time.Since(cacheCreatedAt) > cacheTTL {
		return "", false
	}

	// Verify the cached file still exists
	if _, err := os.Stat(cachedDBPath); os.IsNotExist(err) {
		return "", false
	}

	return cachedDBPath, true
}

// generateDB creates a new SQLite database from PostgreSQL data and caches it
func generateDB() (string, error) {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	// Double-check: another goroutine may have regenerated while we waited for the lock
	if cachedDBPath != "" && time.Since(cacheCreatedAt) <= cacheTTL {
		if _, err := os.Stat(cachedDBPath); err == nil {
			return cachedDBPath, nil
		}
	}

	// Remove old cached file if it exists
	if cachedDBPath != "" {
		os.Remove(cachedDBPath)
	}

	// Create a new file for the SQLite database (not in temp, so it persists)
	appLog.Debug("Creating SQLite database file...")
	tmpFile, err := os.CreateTemp("", "cached-db-*.db")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	// Open SQLite database
	sqliteDB, err := sql.Open("sqlite", tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to open SQLite database: %w", err)
	}

	// Create tables in SQLite
	appLog.Debug("Creating SQLite tables...")
	tableStart := time.Now()
	if err := createSQLiteTables(sqliteDB); err != nil {
		sqliteDB.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to create tables: %w", err)
	}
	appLog.Debug("Tables created in %s", time.Since(tableStart))

	// Copy data from PostgreSQL to SQLite
	appLog.Info("Copying approved_projects from PostgreSQL...")
	copyStart := time.Now()
	projectCount, err := copyApprovedProjects(sqliteDB)
	if err != nil {
		sqliteDB.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to copy approved_projects: %w", err)
	}
	appLog.Info("Copied %d approved_projects in %s", projectCount, time.Since(copyStart))

	appLog.Info("Copying ysws_project_mentions from PostgreSQL...")
	copyStart = time.Now()
	mentionCount, err := copyProjectMentions(sqliteDB)
	if err != nil {
		sqliteDB.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to copy ysws_project_mentions: %w", err)
	}
	appLog.Info("Copied %d ysws_project_mentions in %s", mentionCount, time.Since(copyStart))

	// Close SQLite to flush all data
	sqliteDB.Close()

	// Get file size
	fileInfo, err := os.Stat(tmpPath)
	if err == nil {
		appLog.Info("SQLite database size: %.2f MB, total rows: %d", float64(fileInfo.Size())/(1024*1024), projectCount+mentionCount)
	}

	// Update cache
	cachedDBPath = tmpPath
	cacheCreatedAt = time.Now()

	return tmpPath, nil
}

// serveCachedDB sends the cached database file to the client
func serveCachedDB(w http.ResponseWriter, dbPath string, requestStart time.Time) {
	// Open the file for reading
	file, err := os.Open(dbPath)
	if err != nil {
		appLog.Error("Failed to open file for reading: %v", err)
		http.Error(w, fmt.Sprintf("Failed to open file for reading: %v", err), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// Get file info for size
	fileInfo, err := file.Stat()
	if err != nil {
		appLog.Error("Failed to stat file: %v", err)
		http.Error(w, fmt.Sprintf("Failed to stat file: %v", err), http.StatusInternalServerError)
		return
	}

	// Set headers for file download
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="database.db"`)
	w.Header().Set("Content-Transfer-Encoding", "binary")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", fileInfo.Size()))

	// Copy file contents to response
	bytesSent, err := io.Copy(w, file)
	if err != nil {
		appLog.Error("Error writing response: %v", err)
		return
	}

	appLog.Info("Database sent: %.2f MB in %s", float64(bytesSent)/(1024*1024), time.Since(requestStart))
}

func createSQLiteTables(db *sql.DB) error {
	// Create approved_projects table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS approved_projects (
			record_id TEXT PRIMARY KEY,
			first_name TEXT,
			git_hub_username TEXT,
			geocoded_country TEXT,
			hack_clubber_geocoded_country TEXT,
			geocoded_country_code TEXT,
			playable_url TEXT,
			code_url TEXT
		)
	`)
	if err != nil {
		return fmt.Errorf("creating approved_projects table: %w", err)
	}

	// Create ysws_project_mentions table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS ysws_project_mentions (
			id TEXT PRIMARY KEY,
			ysws_project_mentions_id TEXT,
			ysws_project_mention_searches TEXT,
			ysws_from_ysws_approved_project TEXT,
			record_id TEXT,
			ysws_approved_project TEXT,
			source TEXT,
			link_found_at TEXT,
			archive_url TEXT,
			url TEXT,
			headline TEXT,
			date TEXT,
			weighted_engagement_points REAL,
			project_url TEXT,
			engagement_count INTEGER,
			engagement_type TEXT,
			mentions_hack_club INTEGER,
			published_by_hack_club INTEGER
		)
	`)
	if err != nil {
		return fmt.Errorf("creating ysws_project_mentions table: %w", err)
	}

	// Create indexes for efficient queries
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_mentions_record_id ON ysws_project_mentions(record_id)`)
	if err != nil {
		return fmt.Errorf("creating record_id index: %w", err)
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_mentions_approved_project ON ysws_project_mentions(ysws_approved_project)`)
	if err != nil {
		return fmt.Errorf("creating ysws_approved_project index: %w", err)
	}

	return nil
}

func copyApprovedProjects(sqliteDB *sql.DB) (int, error) {
	// Query PostgreSQL for approved_projects data
	rows, err := pgDB.Query(`
		SELECT 
			record_id,
			first_name,
			git_hub_username,
			geocoded_country,
			hack_clubber_geocoded_country,
			geocoded_country_code,
			playable_url,
			code_url
		FROM airtable_unified_ysws_projects_db.approved_projects
	`)
	if err != nil {
		return 0, fmt.Errorf("querying PostgreSQL: %w", err)
	}
	defer rows.Close()

	// Begin transaction for faster inserts
	tx, err := sqliteDB.Begin()
	if err != nil {
		return 0, fmt.Errorf("beginning transaction: %w", err)
	}

	// Prepare SQLite insert statement
	stmt, err := tx.Prepare(`
		INSERT INTO approved_projects (
			record_id, first_name, git_hub_username, geocoded_country,
			hack_clubber_geocoded_country, geocoded_country_code, playable_url, code_url
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		tx.Rollback()
		return 0, fmt.Errorf("preparing insert statement: %w", err)
	}
	defer stmt.Close()

	count := 0
	for rows.Next() {
		var recordID, firstName, gitHubUsername, geocodedCountry sql.NullString
		var hackClubberGeocodedCountry, geocodedCountryCode, playableURL, codeURL sql.NullString

		err := rows.Scan(
			&recordID, &firstName, &gitHubUsername, &geocodedCountry,
			&hackClubberGeocodedCountry, &geocodedCountryCode, &playableURL, &codeURL,
		)
		if err != nil {
			tx.Rollback()
			return 0, fmt.Errorf("scanning row: %w", err)
		}

		_, err = stmt.Exec(
			nullStringToPtr(recordID), nullStringToPtr(firstName),
			nullStringToPtr(gitHubUsername), nullStringToPtr(geocodedCountry),
			nullStringToPtr(hackClubberGeocodedCountry), nullStringToPtr(geocodedCountryCode),
			nullStringToPtr(playableURL), nullStringToPtr(codeURL),
		)
		if err != nil {
			tx.Rollback()
			return 0, fmt.Errorf("inserting row: %w", err)
		}
		count++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing transaction: %w", err)
	}

	return count, nil
}

func copyProjectMentions(sqliteDB *sql.DB) (int, error) {
	// Query PostgreSQL for ysws_project_mentions data
	rows, err := pgDB.Query(`
		SELECT 
			id,
			ysws_project_mentions_id,
			ysws_project_mention_searches,
			ysws_from_ysws_approved_project,
			record_id,
			ysws_approved_project,
			source,
			link_found_at,
			archive_url,
			url,
			headline,
			date,
			weighted_engagement_points,
			project_url,
			engagement_count,
			engagement_type,
			mentions_hack_club,
			published_by_hack_club
		FROM airtable_unified_ysws_projects_db.ysws_project_mentions
	`)
	if err != nil {
		return 0, fmt.Errorf("querying PostgreSQL: %w", err)
	}
	defer rows.Close()

	// Begin transaction for faster inserts
	tx, err := sqliteDB.Begin()
	if err != nil {
		return 0, fmt.Errorf("beginning transaction: %w", err)
	}

	// Prepare SQLite insert statement
	stmt, err := tx.Prepare(`
		INSERT INTO ysws_project_mentions (
			id, ysws_project_mentions_id, ysws_project_mention_searches,
			ysws_from_ysws_approved_project, record_id, ysws_approved_project,
			source, link_found_at, archive_url, url, headline, date,
			weighted_engagement_points, project_url, engagement_count,
			engagement_type, mentions_hack_club, published_by_hack_club
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		tx.Rollback()
		return 0, fmt.Errorf("preparing insert statement: %w", err)
	}
	defer stmt.Close()

	count := 0
	for rows.Next() {
		var id, mentionsID, mentionSearches, fromApproved sql.NullString
		var recordID, yswsApproved, source, linkFoundAt sql.NullString
		var archiveURL, url, headline, date sql.NullString
		var weightedEngagement sql.NullFloat64
		var projectURL, engagementType sql.NullString
		var engagementCount sql.NullInt64
		var mentionsHackClub, publishedByHackClub sql.NullBool

		err := rows.Scan(
			&id, &mentionsID, &mentionSearches, &fromApproved,
			&recordID, &yswsApproved, &source, &linkFoundAt,
			&archiveURL, &url, &headline, &date,
			&weightedEngagement, &projectURL, &engagementCount,
			&engagementType, &mentionsHackClub, &publishedByHackClub,
		)
		if err != nil {
			tx.Rollback()
			return 0, fmt.Errorf("scanning row: %w", err)
		}

		_, err = stmt.Exec(
			nullStringToPtr(id), nullStringToPtr(mentionsID),
			nullStringToPtr(mentionSearches), nullStringToPtr(fromApproved),
			nullStringToPtr(recordID), nullStringToPtr(yswsApproved),
			nullStringToPtr(source), nullStringToPtr(linkFoundAt),
			nullStringToPtr(archiveURL), nullStringToPtr(url),
			nullStringToPtr(headline), nullStringToPtr(date),
			nullFloat64ToPtr(weightedEngagement), nullStringToPtr(projectURL),
			nullInt64ToPtr(engagementCount), nullStringToPtr(engagementType),
			nullBoolToInt(mentionsHackClub), nullBoolToInt(publishedByHackClub),
		)
		if err != nil {
			tx.Rollback()
			return 0, fmt.Errorf("inserting row: %w", err)
		}
		count++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing transaction: %w", err)
	}

	return count, nil
}

func nullStringToPtr(ns sql.NullString) interface{} {
	if ns.Valid {
		return ns.String
	}
	return nil
}

func nullFloat64ToPtr(nf sql.NullFloat64) interface{} {
	if nf.Valid {
		return nf.Float64
	}
	return nil
}

func nullInt64ToPtr(ni sql.NullInt64) interface{} {
	if ni.Valid {
		return ni.Int64
	}
	return nil
}

func nullBoolToInt(nb sql.NullBool) interface{} {
	if nb.Valid {
		if nb.Bool {
			return 1
		}
		return 0
	}
	return nil
}
