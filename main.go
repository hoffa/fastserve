package main

import (
	"bytes"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
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

func (s *server) loadFiles(ignore regexp.Regexp) error {
	seen := make(map[string]bool)

	if err := filepath.Walk(s.dir, func(path string, info os.FileInfo, err error) error {
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

		if ignore.MatchString(relPath) {
			return nil
		}

		seen[relPath] = true

		s.mu.RLock()
		cached, exists := s.cache[relPath]
		s.mu.RUnlock()

		if exists && info.ModTime().Equal(cached.modTime) {
			return nil
		}

		log.Println("caching", relPath)
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
	}); err != nil {
		return err
	}

	s.mu.Lock()
	for path := range s.cache {
		if !seen[path] {
			log.Println("uncaching", path)
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
	path := r.URL.Path[1:]

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
	addr := flag.String("addr", ":8080", "address to listen on")
	dir := flag.String("dir", ".", "directory to serve")
	refresh := flag.Duration("refresh", time.Minute, "file refresh interval")
	ignorePattern := flag.String("ignore", "^\\.", "file ignore pattern")
	timeout := flag.Duration("timeout", 30*time.Second, "HTTP timeout")
	flag.Parse()

	ignore, err := regexp.Compile(*ignorePattern)
	if err != nil {
		log.Fatal(err)
	}

	srv := newServer(*dir)

	if err := srv.loadFiles(*ignore); err != nil {
		log.Fatal(err)
	}

	go func() {
		for {
			time.Sleep(*refresh)
			start := time.Now()
			if err := srv.loadFiles(*ignore); err != nil {
				log.Fatal(err)
			}
			log.Println("refreshed in", time.Since(start))
		}
	}()

	server := &http.Server{
		Addr:         *addr,
		Handler:      logRequest(srv.handleRequest),
		ReadTimeout:  *timeout,
		WriteTimeout: *timeout,
	}

	log.Printf("serving %s on %s", *dir, *addr)
	log.Fatal(server.ListenAndServe())
}
