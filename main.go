package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
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
        last_accessed_at DATETIME
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
			var shortURL string

			if customAlias != "" {
				// Check if customAlias is provided and unique
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
				}
				shortURL = customAlias
			} else {
				// Generate a unique random short URL
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

			// Insert the URL and short URL (and custom alias if provided) into the database
			_, err = db.Exec("INSERT INTO urls (original_url, short_url, custom_alias) VALUES (?, ?, ?)", originalURL, shortURL, customAlias)
			if err != nil {
				log.Printf("Failed to insert URL: %v", err)
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}

			fmt.Fprintf(w, "URL shortened successfully: %s", shortURL)
		}
	})

	// Handler to redirect to the original URL
	http.HandleFunc("/s/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/s/")
		var originalURL string

		err := db.QueryRow("SELECT original_url FROM urls WHERE short_url = ? OR custom_alias = ?", path, path).Scan(&originalURL)
		if err != nil {
			if err == sql.ErrNoRows {
				http.NotFound(w, r)
			} else {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
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
