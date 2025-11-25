package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
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
	"github.com/klauspost/compress/zstd"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

var (
	apiKey    string
	emailSalt string
	pgDB      *sql.DB

	// Cache for the generated SQLite database (zstd compressed)
	cacheMutex           sync.RWMutex
	cachedCompressedPath string
	cacheCreatedAt       time.Time
	cacheTTL             = 5 * time.Minute
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

// hashEmail normalizes an email (lowercase, strip spaces) and returns an HMAC-SHA256 hash
// using the EMAIL_SALT as the secret key for cryptographic security
func hashEmail(email string) string {
	if email == "" {
		return ""
	}
	// Normalize: lowercase and strip spaces
	normalized := strings.ToLower(strings.TrimSpace(email))
	// Create HMAC-SHA256 using emailSalt as the secret key
	h := hmac.New(sha256.New, []byte(emailSalt))
	h.Write([]byte(normalized))
	return hex.EncodeToString(h.Sum(nil))
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

	// Get email salt from environment variable, or generate one if not set
	emailSalt = os.Getenv("EMAIL_SALT")
	if emailSalt == "" {
		var err error
		emailSalt, err = generateAPIKey() // Reuse the same random generator
		if err != nil {
			appLog.Error("Failed to generate email salt: %v", err)
			os.Exit(1)
		}
		fmt.Println("")
		fmt.Println("=" + strings.Repeat("=", 70) + "=")
		fmt.Println("⚠️  EMAIL_SALT not set in environment")
		fmt.Println("🧂 Generated email salt (save this if you need consistent hashes):")
		fmt.Println("")
		fmt.Printf("   %s\n", emailSalt)
		fmt.Println("=" + strings.Repeat("=", 70) + "=")
		fmt.Println("")
	} else {
		appLog.Info("Using email salt from environment")
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

		if subtle.ConstantTimeCompare([]byte(providedKey), []byte(apiKey)) != 1 {
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
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	appLog.Info("Generated fresh database, caching for %s", cacheTTL)
	serveCachedDB(w, newPath, requestStart)
}

// getCachedDB checks if we have a valid cached compressed database and returns its path
// Returns (path, true) if cache is valid, ("", false) if cache needs refresh
func getCachedDB() (string, bool) {
	cacheMutex.RLock()
	defer cacheMutex.RUnlock()

	// Check if cache exists and is still valid
	if cachedCompressedPath == "" || time.Since(cacheCreatedAt) > cacheTTL {
		return "", false
	}

	// Verify the cached file still exists
	if _, err := os.Stat(cachedCompressedPath); os.IsNotExist(err) {
		return "", false
	}

	return cachedCompressedPath, true
}

// generateDB creates a new SQLite database from PostgreSQL data, compresses it with zstd, and caches it
func generateDB() (string, error) {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	// Double-check: another goroutine may have regenerated while we waited for the lock
	if cachedCompressedPath != "" && time.Since(cacheCreatedAt) <= cacheTTL {
		if _, err := os.Stat(cachedCompressedPath); err == nil {
			return cachedCompressedPath, nil
		}
	}

	// Remove old cached file if it exists
	if cachedCompressedPath != "" {
		os.Remove(cachedCompressedPath)
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

	// Get uncompressed file size
	fileInfo, err := os.Stat(tmpPath)
	var uncompressedSize int64
	if err == nil {
		uncompressedSize = fileInfo.Size()
		appLog.Info("SQLite database size (uncompressed): %.2f MB, total rows: %d", float64(uncompressedSize)/(1024*1024), projectCount+mentionCount)
	}

	// Compress the database with zstd
	appLog.Info("Compressing database with zstd...")
	compressStart := time.Now()
	compressedPath, err := compressWithZstd(tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to compress database: %w", err)
	}

	// Remove the uncompressed file
	os.Remove(tmpPath)

	// Get compressed file size
	compressedInfo, err := os.Stat(compressedPath)
	if err == nil {
		compressedSize := compressedInfo.Size()
		ratio := float64(uncompressedSize) / float64(compressedSize)
		appLog.Info("Compressed database size: %.2f MB (%.1fx compression) in %s",
			float64(compressedSize)/(1024*1024), ratio, time.Since(compressStart))
	}

	// Update cache
	cachedCompressedPath = compressedPath
	cacheCreatedAt = time.Now()

	return compressedPath, nil
}

// compressWithZstd compresses a file using zstd and returns the path to the compressed file
func compressWithZstd(inputPath string) (string, error) {
	// Create output file
	outputPath := inputPath + ".zst"
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("failed to create output file: %w", err)
	}
	defer outputFile.Close()

	// Create zstd encoder with best compression
	encoder, err := zstd.NewWriter(outputFile, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
	if err != nil {
		os.Remove(outputPath)
		return "", fmt.Errorf("failed to create zstd encoder: %w", err)
	}

	// Open input file
	inputFile, err := os.Open(inputPath)
	if err != nil {
		encoder.Close()
		os.Remove(outputPath)
		return "", fmt.Errorf("failed to open input file: %w", err)
	}
	defer inputFile.Close()

	// Copy and compress
	_, err = io.Copy(encoder, inputFile)
	if err != nil {
		encoder.Close()
		os.Remove(outputPath)
		return "", fmt.Errorf("failed to compress: %w", err)
	}

	// Close encoder to flush all data
	if err := encoder.Close(); err != nil {
		os.Remove(outputPath)
		return "", fmt.Errorf("failed to close encoder: %w", err)
	}

	return outputPath, nil
}

// serveCachedDB sends the cached zstd-compressed database file to the client
func serveCachedDB(w http.ResponseWriter, compressedPath string, requestStart time.Time) {
	// Open the file for reading
	file, err := os.Open(compressedPath)
	if err != nil {
		appLog.Error("Failed to open file for reading: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// Get file info for size
	fileInfo, err := file.Stat()
	if err != nil {
		appLog.Error("Failed to stat file: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Set headers for zstd-compressed file download
	w.Header().Set("Content-Type", "application/zstd")
	w.Header().Set("Content-Disposition", `attachment; filename="database.db.zst"`)
	w.Header().Set("Content-Transfer-Encoding", "binary")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", fileInfo.Size()))

	// Copy file contents to response
	bytesSent, err := io.Copy(w, file)
	if err != nil {
		appLog.Error("Error writing response: %v", err)
		return
	}

	appLog.Info("Compressed database sent: %.2f MB in %s", float64(bytesSent)/(1024*1024), time.Since(requestStart))
}

func createSQLiteTables(db *sql.DB) error {
	// Create approved_projects table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS approved_projects (
			record_id TEXT PRIMARY KEY,
			first_name TEXT,
			last_name TEXT,
			git_hub_username TEXT,
			geocoded_country TEXT,
			geocoded_country_code TEXT,
			playable_url TEXT,
			code_url TEXT,
			hours_spent REAL,
			approved_at TEXT,
			override_hours_spent_justification TEXT,
			age_when_approved INTEGER,
			ysws_name TEXT,
			email_hash TEXT
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
	// Query PostgreSQL for approved_projects data with YSWS name from child table
	rows, err := pgDB.Query(`
		SELECT 
			ap.record_id,
			ap.first_name,
			ap.last_name,
			ap.git_hub_username,
			ap.geocoded_country,
			ap.geocoded_country_code,
			ap.playable_url,
			ap.code_url,
			ap.hours_spent,
			ap.approved_at,
			ap.override_hours_spent_justification,
			ap.age_when_approved,
			ysws_name.value as ysws_name,
			ap.email
		FROM airtable_unified_ysws_projects_db.approved_projects ap
		LEFT JOIN airtable_unified_ysws_projects_db.approved_projects__ysws_name ysws_name
			ON ap._dlt_id = ysws_name._dlt_parent_id
			AND ysws_name._dlt_list_idx = 0
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
			record_id, first_name, last_name, git_hub_username, geocoded_country,
			geocoded_country_code, playable_url, code_url,
			hours_spent, approved_at, override_hours_spent_justification, age_when_approved,
			ysws_name, email_hash
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		tx.Rollback()
		return 0, fmt.Errorf("preparing insert statement: %w", err)
	}
	defer stmt.Close()

	count := 0
	for rows.Next() {
		var recordID, firstName, lastName, gitHubUsername, geocodedCountry sql.NullString
		var geocodedCountryCode, playableURL, codeURL sql.NullString
		var hoursSpent sql.NullFloat64
		var approvedAt, overrideHoursJustification sql.NullString
		var ageWhenApproved sql.NullInt64
		var yswsName sql.NullString
		var email sql.NullString

		err := rows.Scan(
			&recordID, &firstName, &lastName, &gitHubUsername, &geocodedCountry,
			&geocodedCountryCode, &playableURL, &codeURL,
			&hoursSpent, &approvedAt, &overrideHoursJustification, &ageWhenApproved,
			&yswsName, &email,
		)
		if err != nil {
			tx.Rollback()
			return 0, fmt.Errorf("scanning row: %w", err)
		}

		// Hash the email if present
		var emailHash *string
		if email.Valid && email.String != "" {
			h := hashEmail(email.String)
			emailHash = &h
		}

		_, err = stmt.Exec(
			nullStringToPtr(recordID), nullStringToPtr(firstName),
			nullStringToPtr(lastName), nullStringToPtr(gitHubUsername), nullStringToPtr(geocodedCountry),
			nullStringToPtr(geocodedCountryCode),
			normalizeURL(playableURL), normalizeURL(codeURL),
			nullFloat64ToPtr(hoursSpent), nullStringToPtr(approvedAt),
			nullStringToPtr(overrideHoursJustification), nullInt64ToPtr(ageWhenApproved),
			nullStringToPtr(yswsName), emailHash,
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
			normalizeURL(archiveURL), normalizeURL(url),
			nullStringToPtr(headline), nullStringToPtr(date),
			nullFloat64ToPtr(weightedEngagement), normalizeURL(projectURL),
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

// dangerousSchemes contains URL schemes that should be rejected for security reasons.
// These schemes can be used for XSS attacks if URLs are rendered in HTML contexts.
var dangerousSchemes = []string{
	"javascript:",
	"data:",
	"vbscript:",
	"file:",
}

// normalizeURL normalizes a URL by:
// - Trimming whitespace
// - Lowercasing
// - Rejecting dangerous URL schemes (javascript:, data:, vbscript:, file:)
// - Adding https:// prefix if no scheme is present
// - Removing .git suffix (for GitHub clone URLs)
// - Removing /tree/... paths from GitHub URLs (branch references)
// - Removing trailing slashes (so /repo and /repo/ are treated the same)
func normalizeURL(ns sql.NullString) interface{} {
	if !ns.Valid || ns.String == "" {
		return nil
	}

	// Trim whitespace and normalize multiple spaces
	url := strings.TrimSpace(ns.String)
	// Replace multiple spaces with single space, then remove all spaces
	url = strings.Join(strings.Fields(url), "")

	// Lowercase the URL for consistent comparison
	url = strings.ToLower(url)

	// Reject dangerous URL schemes (must be done after lowercasing to catch all case variations)
	for _, scheme := range dangerousSchemes {
		if strings.HasPrefix(url, scheme) {
			return nil
		}
	}

	// Add https:// if no scheme present
	if url != "" && !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}

	// Remove trailing slashes for consistent comparison
	// (e.g., github.com/user/repo/ and github.com/user/repo should be the same)
	// This must happen before .git removal so that .git/ is handled correctly
	url = strings.TrimRight(url, "/")

	// Remove .git suffix (common in GitHub clone URLs)
	url = strings.TrimSuffix(url, ".git")

	// Remove /tree/... paths from GitHub URLs (these are branch/tag references, not file paths)
	// Keep /blob/... paths intact as they reference specific files
	if strings.Contains(url, "github.com/") {
		if idx := strings.Index(url, "/tree/"); idx != -1 {
			url = url[:idx]
		}
	}

	if url == "" {
		return nil
	}

	return url
}
