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

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

var (
	apiKey string
	pgDB   *sql.DB
)

func generateAPIKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func main() {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		if err2 := godotenv.Load("../.env"); err2 != nil {
			log.Printf("Warning: .env file not found or couldn't be loaded: %v", err)
		}
	}

	// Get API key from environment variable, or generate one if not set
	apiKey = os.Getenv("API_KEY")
	if apiKey == "" {
		var err error
		apiKey, err = generateAPIKey()
		if err != nil {
			log.Fatalf("Failed to generate API key: %v", err)
		}
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
	}

	// Connect to PostgreSQL
	dbURL := os.Getenv("WAREHOUSE_READONLY_UNIFIED_YSWS_DATABASE_URL")
	if dbURL == "" {
		log.Fatal("WAREHOUSE_READONLY_UNIFIED_YSWS_DATABASE_URL environment variable is required")
	}

	fmt.Println("Connecting to PostgreSQL...")
	var err error
	pgDB, err = sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to open PostgreSQL connection: %v", err)
	}
	defer pgDB.Close()

	if err := pgDB.Ping(); err != nil {
		log.Fatalf("Failed to ping PostgreSQL database: %v", err)
	}
	fmt.Println("✓ Connected to PostgreSQL database")

	// Create a mux to handle all routes with authentication
	mux := http.NewServeMux()
	mux.HandleFunc("/db", dbHandler)

	handler := authMiddleware(mux)

	port := ":8080"
	fmt.Printf("Server starting on port %s\n", port)
	fmt.Printf("API key authentication is enabled\n")
	fmt.Printf("Visit http://localhost%s/db to download the SQLite database\n", port)

	if err := http.ListenAndServe(port, handler); err != nil {
		log.Fatal(err)
	}
}

func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		apiKeyHeader := r.Header.Get("X-API-Key")

		var providedKey string
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
				providedKey = parts[1]
			} else {
				providedKey = authHeader
			}
		} else if apiKeyHeader != "" {
			providedKey = apiKeyHeader
		}

		if providedKey == "" || providedKey != apiKey {
			w.Header().Set("WWW-Authenticate", `Bearer realm="API"`)
			http.Error(w, "Unauthorized: API key is required", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func dbHandler(w http.ResponseWriter, r *http.Request) {
	// Create a temporary file for the SQLite database
	tmpFile, err := os.CreateTemp("", "db-*.db")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create temp file: %v", err), http.StatusInternalServerError)
		return
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Open SQLite database
	sqliteDB, err := sql.Open("sqlite", tmpPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to open SQLite database: %v", err), http.StatusInternalServerError)
		return
	}
	defer sqliteDB.Close()

	// Create tables in SQLite
	if err := createSQLiteTables(sqliteDB); err != nil {
		http.Error(w, fmt.Sprintf("Failed to create tables: %v", err), http.StatusInternalServerError)
		return
	}

	// Copy data from PostgreSQL to SQLite
	if err := copyApprovedProjects(sqliteDB); err != nil {
		http.Error(w, fmt.Sprintf("Failed to copy approved_projects: %v", err), http.StatusInternalServerError)
		return
	}

	if err := copyProjectMentions(sqliteDB); err != nil {
		http.Error(w, fmt.Sprintf("Failed to copy ysws_project_mentions: %v", err), http.StatusInternalServerError)
		return
	}

	// Close SQLite to flush all data
	sqliteDB.Close()

	// Open the file for reading
	file, err := os.Open(tmpPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to open file for reading: %v", err), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// Set headers for file download
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="database.db"`)
	w.Header().Set("Content-Transfer-Encoding", "binary")

	// Copy file contents to response
	if _, err := io.Copy(w, file); err != nil {
		log.Printf("Error writing response: %v", err)
	}
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

	// Create index for joining tables
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_mentions_record_id ON ysws_project_mentions(record_id)`)
	if err != nil {
		return fmt.Errorf("creating index: %w", err)
	}

	return nil
}

func copyApprovedProjects(sqliteDB *sql.DB) error {
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
		return fmt.Errorf("querying PostgreSQL: %w", err)
	}
	defer rows.Close()

	// Prepare SQLite insert statement
	stmt, err := sqliteDB.Prepare(`
		INSERT INTO approved_projects (
			record_id, first_name, git_hub_username, geocoded_country,
			hack_clubber_geocoded_country, geocoded_country_code, playable_url, code_url
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("preparing insert statement: %w", err)
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
			return fmt.Errorf("scanning row: %w", err)
		}

		_, err = stmt.Exec(
			nullStringToPtr(recordID), nullStringToPtr(firstName),
			nullStringToPtr(gitHubUsername), nullStringToPtr(geocodedCountry),
			nullStringToPtr(hackClubberGeocodedCountry), nullStringToPtr(geocodedCountryCode),
			nullStringToPtr(playableURL), nullStringToPtr(codeURL),
		)
		if err != nil {
			return fmt.Errorf("inserting row: %w", err)
		}
		count++
	}

	log.Printf("Copied %d rows to approved_projects", count)
	return nil
}

func copyProjectMentions(sqliteDB *sql.DB) error {
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
		return fmt.Errorf("querying PostgreSQL: %w", err)
	}
	defer rows.Close()

	// Prepare SQLite insert statement
	stmt, err := sqliteDB.Prepare(`
		INSERT INTO ysws_project_mentions (
			id, ysws_project_mentions_id, ysws_project_mention_searches,
			ysws_from_ysws_approved_project, record_id, ysws_approved_project,
			source, link_found_at, archive_url, url, headline, date,
			weighted_engagement_points, project_url, engagement_count,
			engagement_type, mentions_hack_club, published_by_hack_club
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("preparing insert statement: %w", err)
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
			return fmt.Errorf("scanning row: %w", err)
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
			return fmt.Errorf("inserting row: %w", err)
		}
		count++
	}

	log.Printf("Copied %d rows to ysws_project_mentions", count)
	return nil
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
