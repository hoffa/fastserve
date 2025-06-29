package main

import (
	"bytes"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
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
	tempCache := make(map[string]*fileCache)
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

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		tempCache[relPath] = &fileCache{
			content: content,
			modTime: info.ModTime(),
		}

		return nil
	})

	if err != nil {
		return err
	}

	s.mu.Lock()
	s.cache = tempCache
	s.mu.Unlock()

	return nil
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

	http.HandleFunc("/", srv.handleRequest)

	log.Printf("Serving %s on %s (refreshing every %v)", *dir, *addr, *refresh)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
