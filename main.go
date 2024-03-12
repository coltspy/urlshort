package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	type ExpirationOption struct {
		Name     string
		Duration time.Duration
	}

	var expirationOptions = []ExpirationOption{
		{"1 Day", 24 * time.Hour},
		{"1 Month", 30 * 24 * time.Hour},
		{"1 Year", 365 * 24 * time.Hour},
		{"Lifetime", 0},
	}

	// Connect to SQLite Database
	db, err := sql.Open("sqlite3", "urlshortener.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Create the URLs table if it doesn't exist
	createTableSQL := `CREATE TABLE IF NOT EXISTS urls (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        short_url TEXT,
        custom_alias TEXT UNIQUE,
        original_url TEXT,
        access_count INTEGER DEFAULT 0,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
        last_accessed_at DATETIME,
		expires_at DATETIME
    );`
	_, err = db.Exec(createTableSQL)
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})

	// Handler to shorten URLs
	http.HandleFunc("/shorten", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			r.ParseForm()
			originalURL := r.FormValue("url")
			customAlias := r.FormValue("customAlias")
			expirationOption := r.FormValue("expiration")

			var shortURL string

			if customAlias != "" {
				var exists int
				err := db.QueryRow("SELECT COUNT(*) FROM urls WHERE custom_alias = ?", customAlias).Scan(&exists)
				if err != nil {
					log.Printf("Failed to check custom alias: %v", err)
					http.Error(w, "Database error", http.StatusInternalServerError)
					return
				}
				if exists > 0 {
					http.Error(w, "Custom alias already in use", http.StatusBadRequest)
					return
				} else {
					shortURL = customAlias
				}
			} else {
				for {
					shortURL, err = generateRandomString(8) // Generate an 8 character string
					if err != nil {
						log.Printf("Failed to generate a random string: %v", err)
						http.Error(w, "Failed to generate a short URL", http.StatusInternalServerError)
						return
					}

					var exists int
					err = db.QueryRow("SELECT COUNT(*) FROM urls WHERE short_url = ?", shortURL).Scan(&exists)
					if err != nil {
						log.Printf("Failed to check short URL: %v", err)
						http.Error(w, "Database error", http.StatusInternalServerError)
						return
					}
					if exists == 0 {
						break // Unique short URL found
					}
				}
			}
			var expirationDuration time.Duration
			for _, option := range expirationOptions {
				if option.Name == expirationOption {
					expirationDuration = option.Duration
					break
				}
			}

			expiresAt := time.Now().Add(expirationDuration)

			// Insert the URL and short URL (and custom alias if provided) into the database
			_, err = db.Exec("INSERT INTO urls (original_url, short_url, custom_alias, expires_at) VALUES (?, ?, ?, ?)", originalURL, shortURL, customAlias, expiresAt) // Note the addition of expiresAt
			if err != nil {
				log.Printf("Failed to insert URL: %v", err)
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}

			fullURL := "https://urlshort-five.vercel.app/s/" + shortURL
			fmt.Fprintf(w, "Shortened URL: <a href=\"%s\" target=\"_blank\">%s</a>", fullURL, fullURL)

		}
	})

	// Handler to redirect to the original URL
	http.HandleFunc("/s/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/s/")
		log.Printf("Requested path: %s", path) // Debugging log

		var originalURL string
		var expiresAt sql.NullTime // Use sql.NullTime to handle possible NULL values

		err := db.QueryRow("SELECT original_url, expires_at FROM urls WHERE short_url = ? OR custom_alias = ?", path, path).Scan(&originalURL, &expiresAt)
		if err != nil {
			log.Printf("Database query error: %v", err) // Debugging log
			if err == sql.ErrNoRows {
				http.NotFound(w, r)
			} else {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}

		if expiresAt.Valid && time.Now().After(expiresAt.Time) {
			http.Error(w, "This URL has expired", http.StatusGone)
			return
		}

		// Increment access count and update last accessed timestamp
		_, err = db.Exec("UPDATE urls SET access_count = access_count + 1, last_accessed_at = CURRENT_TIMESTAMP WHERE short_url = ? OR custom_alias = ?", path, path)
		if err != nil {
			log.Printf("Failed to update access count and timestamp for %s: %v", path, err)
		}

		http.Redirect(w, r, originalURL, http.StatusFound)
	})

	// Start the HTTP server
	log.Println("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func generateRandomString(n int) (string, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b)[:n], nil
}
