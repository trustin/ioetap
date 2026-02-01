# ioetap (In/Out/Err Tap)

A Go CLI application that spawns a target command, forwards all stdio, and records I/O traffic to an NDJSON file.

## Installation

### From source

```bash
go install github.com/trustin/ioetap/cmd/ioetap@latest
```

### Download binaries

Pre-built binaries are available for:
- macOS ARM64 (Apple Silicon)
- Linux AMD64

## Usage

```bash
ioetap [options] -- <command> [args...]
ioetap <command> [args...]  # backward compatible (no options)
```

### Options

| Option | Description |
|--------|-------------|
| `--out=<file>` | Output file path (default: `<basename>-<pid>.jsonl`) |
| `--max-line-length=<n>` | Maximum bytes per recorded line. Lines exceeding this limit are truncated and marked with `"truncated": true`. Set to `0` for unlimited. (default: 16 MiB) |
| `--version`, `-v` | Show version information and exit |

### Examples

```bash
# Record a shell session
ioetap bash

# Record the output of a build command
ioetap make build

# Record an interactive Python session
ioetap python3

# Record curl output
ioetap curl -s https://api.example.com/data

# Specify custom output file
ioetap --out=session.jsonl -- bash

# Limit line length to 1KB (useful for commands that may output very long lines)
ioetap --max-line-length=1024 -- cat /var/log/syslog

# Disable line length limit (unlimited)
ioetap --max-line-length=0 -- ./my-program
```

The recording file is saved in the current working directory with the naming convention:
```
<command-basename>-<pid>.jsonl
```

For example, running `ioetap python3` might create `python3-12345.jsonl`.

## Recording Format

The recording file is in NDJSON (Newline Delimited JSON) format, with one record per line. Each record represents a complete line of I/O (delimited by newline characters).

### Record Schema

> **JSON Schema**: [`record-schema.json`](record-schema.json)

```json
{
  "seq": 0,
  "timestamp": "2024-01-15T10:30:45.123Z",
  "source": "stdout",
  "content": "Hello, World!",
  "encoding": "text",
  "end": "\n"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `seq` | number | Sequence number, starts from 0, atomically incremented |
| `timestamp` | string | UTC timestamp with millisecond precision |
| `source` | string | One of: `stdin`, `stdout`, `stderr` |
| `content` | any | The recorded content (format depends on `encoding`) |
| `encoding` | string | One of: `text`, `json`, or `base64` |
| `end` | string | Line ending characters (`\n` or `\r\n`). Omitted if the line has no trailing newline (e.g., final incomplete line at EOF). |
| `truncated` | boolean | Present and `true` only when the line was truncated due to `--max-line-length`. Omitted when not truncated. |

### Content Encoding

Content encoding is automatically detected with the following priority:

1. **json**: Used when the entire line is valid JSON (object, array, number, string, boolean, or null). The content is stored as a native JSON value, not a string.
   ```json
   {"seq": 0, "source": "stdout", "content": {"key": "value"}, "encoding": "json", "end": "\n"}
   ```

2. **text**: Used when the data is valid UTF-8 but not valid JSON. The content is stored as a string.
   ```json
   {"seq": 0, "source": "stdout", "content": "Hello, World!", "encoding": "text", "end": "\n"}
   ```

3. **base64**: Used when the data contains invalid UTF-8 sequences (binary data). The content is base64-encoded.
   ```json
   {"seq": 0, "source": "stdout", "content": "//4AAQ==", "encoding": "base64"}
   ```

### Truncated Records

When a line exceeds the `--max-line-length` limit, it is truncated and marked:

```json
{
  "seq": 0,
  "timestamp": "2024-01-15T10:30:45.123Z",
  "source": "stdout",
  "content": "This is a very long line that was trun",
  "encoding": "text",
  "end": "\n",
  "truncated": true
}
```

The `truncated` field is only present when `true`. The content contains exactly `--max-line-length` bytes of the original line, and the line ending is preserved in the `end` field.

## Signal Handling

ioetap forwards the following signals to the child process:
- SIGINT (Ctrl+C)
- SIGTERM
- SIGHUP
- SIGQUIT
- SIGUSR1
- SIGUSR2

The child process's exit code is propagated to the parent.

## License

[MIT License](LICENSE.md)

---

## Building from Source

```bash
# Build for current platform (includes version info)
make build

# Run all tests
make test

# Build and test
make all

# Cross-compile for macOS ARM64 and Linux AMD64
make cross-compile

# Build release binaries with a specific version
make release VERSION=1.0.0

# Clean build artifacts
make clean
```

### Version Information

The binary includes version information that can be viewed with `--version`:

```bash
$ ioetap --version
ioetap 1.0.0-dev (abc1234) built 2024-01-15T10:30:45Z linux/amd64
```

Version info is injected at build time via `ldflags`. The Makefile automatically:
- Reads the base version from `internal/version/version.go`
- Gets the git commit hash
- Records the build timestamp

For release builds, override the version:
```bash
make release VERSION=1.0.0
```

## Architecture (Developer Notes)

### Package Structure

```
cmd/ioetap/          # Main entry point
internal/
  cli/               # Command-line argument parsing
  process/           # Child process management and signal forwarding
  recorder/          # I/O recording logic
  version/           # Version information (injected at build time)
test/                # Integration tests
```

### Key Components

#### CLI Parser (`internal/cli/parser.go`)

Handles command-line argument parsing with support for:
- `--out=<file>` or `--out <file>` syntax
- `--max-line-length=<n>` or `--max-line-length <n>` syntax
- Backward compatibility mode (no `--` separator required when no options)
- Validation and error messages

Default values are defined as constants (e.g., `DefaultMaxLineLength = 16 * 1024 * 1024`).

#### Record (`internal/recorder/record.go`)

Defines the `Record` struct representing a single I/O record:
- Automatic encoding detection (JSON > text > base64)
- Custom JSON marshaling/unmarshaling for proper field handling
- Line ending extraction (`end` field)
- Truncation marking (`truncated` field)

#### Recorder (`internal/recorder/recorder.go`)

Thread-safe recorder that:
- Buffers incomplete lines until newline is received
- Handles concurrent writes from stdin, stdout, and stderr
- Enforces line length limits with truncation
- Writes NDJSON format to output file

**Truncation Logic:**
1. When buffered data exceeds `maxLineLength`, truncate to limit and enter "truncation mode"
2. In truncation mode, skip incoming bytes until newline is found
3. When newline is found, write the truncated record with `truncated: true`
4. Reset state and continue normal processing

#### Version (`internal/version/version.go`)

Provides version information with build-time injection:
- `Version`: Semantic version (default: `1.0.0-dev`, overridden for releases)
- `GitCommit`: Short git commit hash
- `BuildTime`: UTC build timestamp

Values are injected via `ldflags` during build. See Makefile for details.

### Adding New CLI Options

1. Add field to `Options` struct in `internal/cli/parser.go`
2. Set default value in `Parse()` function
3. Add parsing logic in `parseOptions()` for both `--key=value` and `--key value` formats
4. Update `isKnownOption()` to recognize the new option
5. Update help text in `cmd/ioetap/main.go`
6. Wire the option in `main.go`
7. Add tests in `internal/cli/parser_test.go`

### Running Tests

```bash
# Run all tests
go test ./...

# Run with race detector
go test -race ./...

# Run specific package tests
go test ./internal/cli/...       # Parser tests
go test ./internal/recorder/...  # Recorder and record tests
go test ./test/...               # Integration tests

# Run with verbose output
go test -v ./...
```
