package main

import (
	"bytes"
	"crypto/tls"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

type fileCache struct {
	content []byte
	modTime time.Time
}

type server struct {
	mu    sync.RWMutex
	dir   string
	cache map[string]*fileCache
}

func newServer(dir string) *server {
	return &server{
		dir:   dir,
		cache: make(map[string]*fileCache),
	}
}

func (s *server) loadFiles() error {
	// Track which files we've seen in this scan
	seen := make(map[string]bool)

	err := filepath.Walk(s.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(s.dir, path)
		if err != nil {
			return err
		}

		// Mark this file as seen
		seen[relPath] = true

		s.mu.RLock()
		cached, exists := s.cache[relPath]
		s.mu.RUnlock()

		// If file exists and is unchanged, we're done
		if exists && info.ModTime().Equal(cached.modTime) {
			return nil
		}

		// File is new or modified, read it
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		s.mu.Lock()
		s.cache[relPath] = &fileCache{
			content: content,
			modTime: info.ModTime(),
		}
		s.mu.Unlock()

		return nil
	})

	if err != nil {
		return err
	}

	// Remove files that no longer exist
	s.mu.Lock()
	for path := range s.cache {
		if !seen[path] {
			delete(s.cache, path)
		}
	}
	s.mu.Unlock()

	return nil
}

// logRequest logs HTTP requests with method, path, and status code
func logRequest(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %v", r.Method, r.URL.Path, time.Since(start))
	}
}

func (s *server) handleRequest(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[1:] // Remove leading slash

	s.mu.RLock()
	cached, exists := s.cache[path]
	s.mu.RUnlock()

	if !exists {
		http.NotFound(w, r)
		return
	}

	http.ServeContent(w, r, path, cached.modTime, bytes.NewReader(cached.content))
}

func main() {
	dir := flag.String("dir", ".", "Directory to serve (required)")
	addr := flag.String("addr", ":8080", "Address to listen on")
	refresh := flag.Duration("refresh", time.Minute, "Refresh interval")
	domain := flag.String("https", "", "Enable HTTPS with this domain (e.g., example.com)")
	flag.Parse()

	srv := newServer(*dir)

	// Initial load
	if err := srv.loadFiles(); err != nil {
		log.Fatalf("Failed to load files: %v", err)
	}

	// Start refresh goroutine
	go func() {
		for {
			time.Sleep(*refresh)
			if err := srv.loadFiles(); err != nil {
				log.Printf("Error refreshing files: %v", err)
			}
		}
	}()

	// Wrap handler with logging middleware
	http.HandleFunc("/", logRequest(srv.handleRequest))

	log.Printf("Serving %s (refreshing every %v)", *dir, *refresh)

	if *domain != "" {
		// HTTPS mode with Let's Encrypt
		log.Printf("Starting HTTPS server for https://%s", *domain)

		certManager := autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(*domain),
			Cache:      autocert.DirCache("certs"),
			Email:      "admin@" + *domain, // Optional but recommended
		}

		// HTTP server for redirect and challenges
		go func() {
			log.Printf("Starting HTTP->HTTPS redirect on :80")
			log.Fatal(http.ListenAndServe(":http", certManager.HTTPHandler(nil)))
		}()

		// HTTPS server
		server := &http.Server{
			Addr: ":https",
			TLSConfig: &tls.Config{
				GetCertificate: certManager.GetCertificate,
				MinVersion:     tls.VersionTLS12, // Enforce modern TLS
			},
		}

		log.Fatal(server.ListenAndServeTLS("",
			"")) // Cert and key come from Let's Encrypt
	} else {
		// HTTP only mode
		log.Printf("Starting HTTP server on %s", *addr)
		log.Fatal(http.ListenAndServe(*addr, nil))
	}
}
