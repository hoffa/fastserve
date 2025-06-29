# FastServe - In-Memory File Server

FastServe is a simple HTTP server that serves files from memory with automatic refresh capabilities. It's designed for development environments where you need to serve static files with minimal overhead.

## Features

- 🚀 Blazing fast file serving from memory
- 🔄 Automatic file system watching with configurable refresh intervals
- 🔒 Thread-safe operations with proper locking
- 📦 Handles file additions, modifications, and deletions
- ⚡ Built with Go for maximum performance

## Installation

```bash
go install github.com/yourusername/fastserve@latest
```

Or clone and build manually:

```bash
git clone https://github.com/yourusername/fastserve.git
cd fastserve
go build -o fastserve
```

## Usage

```bash
# Serve files from a directory (default port :8080, refresh every 1 minute)
fastserve -dir /path/to/your/files

# Customize address and refresh interval
fastserve -dir /path/to/your/files -addr :3000 -refresh 30s
```

### Command Line Flags

- `-dir` (required): Directory to serve files from
- `-addr` (default: ":8080"): Address to listen on
- `-refresh` (default: "1m"): Refresh interval (e.g., "30s", "5m")

## Development

### Building

```bash
go build -o fastserve
```

### Testing

```bash
go test -v ./...
```

## License

MIT fastserve