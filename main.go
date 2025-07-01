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

		seen[relPath] = true

		s.mu.RLock()
		cached, exists := s.cache[relPath]
		s.mu.RUnlock()

		if exists && info.ModTime().Equal(cached.modTime) {
			return nil
		}

		log.Println("caching", path)
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

	s.mu.Lock()
	for path := range s.cache {
		if !seen[path] {
			delete(s.cache, path)
		}
	}
	s.mu.Unlock()

	return nil
}

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
	dir := flag.String("dir", ".", "Directory to serve")
	addr := flag.String("addr", ":8080", "Address to listen on")
	refresh := flag.Duration("refresh", time.Minute, "File refresh interval")
	flag.Parse()

	srv := newServer(*dir)

	if err := srv.loadFiles(); err != nil {
		log.Fatal(err)
	}

	go func() {
		for {
			time.Sleep(*refresh)
			if err := srv.loadFiles(); err != nil {
				log.Fatal(err)
			}
		}
	}()

	http.HandleFunc("/", logRequest(srv.handleRequest))

	log.Printf("serving %s on %s (refreshing every %v)", *dir, *addr, *refresh)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
