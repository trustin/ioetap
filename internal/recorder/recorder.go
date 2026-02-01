package recorder

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// Source represents the I/O source type.
type Source int

const (
	Stdin Source = iota
	Stdout
	Stderr
)

// String returns the string representation of the source.
func (s Source) String() string {
	switch s {
	case Stdin:
		return "stdin"
	case Stdout:
		return "stdout"
	case Stderr:
		return "stderr"
	default:
		return "unknown"
	}
}

// Recorder handles thread-safe recording of I/O to an NDJSON file.
// It buffers incomplete lines until a newline is received.
type Recorder struct {
	seq           atomic.Uint64
	file          *os.File
	writer        *bufio.Writer
	mu            sync.Mutex
	buffers       [3][]byte // line buffers indexed by Source (Stdin, Stdout, Stderr)
	truncated     [3]bool   // true if current buffer was truncated
	maxLineLength int       // 0 = unlimited
}

// NewRecorder creates a new Recorder that writes to the specified file.
// maxLineLength limits the maximum bytes per recorded line (0 = unlimited).
func NewRecorder(filename string, maxLineLength int) (*Recorder, error) {
	file, err := os.Create(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to create recording file: %w", err)
	}

	return &Recorder{
		file:          file,
		writer:        bufio.NewWriter(file),
		maxLineLength: maxLineLength,
	}, nil
}

// Record records data from the given source.
// Incomplete lines are buffered until a newline is received.
// Complete lines (ending with \n or \r\n) are written as separate records.
// Lines exceeding maxLineLength are truncated and marked as truncated.
// This method is thread-safe.
func (r *Recorder) Record(source Source, data []byte) error {
	if len(data) == 0 {
		return nil
	}

	now := time.Now()

	r.mu.Lock()
	defer r.mu.Unlock()

	buf := r.buffers[source]
	isTruncated := r.truncated[source]

	for len(data) > 0 {
		idx := bytes.IndexByte(data, '\n')

		if isTruncated {
			// Currently in truncation mode - skip until newline
			if idx == -1 {
				// No newline, skip all remaining data
				return nil
			}
			// Found newline - write truncated record
			lineEnd := idx + 1
			lineEnding := extractLineEnding(buf, data[:lineEnd])
			if err := r.writeTruncatedRecord(now, source, buf, lineEnding); err != nil {
				return err
			}
			r.buffers[source] = nil
			r.truncated[source] = false
			buf = nil
			isTruncated = false
			data = data[lineEnd:]
			continue
		}

		if idx == -1 {
			// No newline found - append to buffer (with truncation check)
			newBuf := append(buf, data...)
			if r.maxLineLength > 0 && len(newBuf) > r.maxLineLength {
				// Truncate to limit
				r.buffers[source] = newBuf[:r.maxLineLength]
				r.truncated[source] = true
			} else {
				r.buffers[source] = newBuf
			}
			return nil
		}

		// Found newline - write complete line
		lineEnd := idx + 1
		var line []byte
		if len(buf) > 0 {
			// Prepend buffer to this line
			line = append(buf, data[:lineEnd]...)
			buf = nil
			r.buffers[source] = nil
		} else {
			// No buffer - use slice directly
			line = data[:lineEnd]
		}

		// Check if line exceeds max length
		if r.maxLineLength > 0 && len(line) > r.maxLineLength {
			lineEnding := extractLineEndingFromLine(line)
			truncatedContent := line[:r.maxLineLength]
			if err := r.writeTruncatedRecord(now, source, truncatedContent, lineEnding); err != nil {
				return err
			}
		} else {
			if err := r.writeRecord(now, source, line, false); err != nil {
				return err
			}
		}
		data = data[lineEnd:]
	}

	return nil
}

// extractLineEnding extracts the line ending (\n or \r\n) from the end of the line.
func extractLineEnding(buf, chunk []byte) []byte {
	combined := append(buf, chunk...)
	return extractLineEndingFromLine(combined)
}

// extractLineEndingFromLine extracts the line ending from a complete line.
func extractLineEndingFromLine(line []byte) []byte {
	if len(line) == 0 {
		return nil
	}
	if line[len(line)-1] != '\n' {
		return nil
	}
	if len(line) >= 2 && line[len(line)-2] == '\r' {
		return []byte{'\r', '\n'}
	}
	return []byte{'\n'}
}

// Flush writes any buffered incomplete line for the given source.
// Call this when the source stream ends (EOF).
// This method is thread-safe.
func (r *Recorder) Flush(source Source) error {
	now := time.Now()

	r.mu.Lock()
	defer r.mu.Unlock()

	buf := r.buffers[source]
	if len(buf) == 0 {
		r.truncated[source] = false
		return nil
	}

	isTruncated := r.truncated[source]
	r.buffers[source] = nil
	r.truncated[source] = false

	if isTruncated {
		return r.writeTruncatedRecord(now, source, buf, nil)
	}
	return r.writeRecord(now, source, buf, false)
}

// writeRecord writes a single record. Must be called with mu held.
func (r *Recorder) writeRecord(now time.Time, source Source, data []byte, truncated bool) error {
	seq := r.seq.Add(1) - 1
	record := NewRecord(seq, now, source.String(), data)
	record.Truncated = truncated

	jsonData, err := record.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize record: %w", err)
	}

	if _, err := r.writer.Write(jsonData); err != nil {
		return fmt.Errorf("failed to write record: %w", err)
	}
	if _, err := r.writer.WriteString("\n"); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	return nil
}

// writeTruncatedRecord writes a truncated record. Must be called with mu held.
// The lineEnding is appended to content for proper End field extraction.
func (r *Recorder) writeTruncatedRecord(now time.Time, source Source, content []byte, lineEnding []byte) error {
	// Append line ending to content so NewRecord can extract it properly
	data := append(content, lineEnding...)
	return r.writeRecord(now, source, data, true)
}

// CopyAndRecord copies data from reader to writer while recording each chunk.
// It returns when the reader reaches EOF or an error occurs.
// Any incomplete line is flushed at EOF.
func (r *Recorder) CopyAndRecord(source Source, reader io.Reader, writer io.Writer) error {
	buf := make([]byte, 32*1024) // 32KB buffer

	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			data := buf[:n]

			// Write to destination
			if _, writeErr := writer.Write(data); writeErr != nil {
				return fmt.Errorf("write error: %w", writeErr)
			}

			// Record the data (log errors but don't fail)
			if recordErr := r.Record(source, data); recordErr != nil {
				fmt.Fprintf(os.Stderr, "ioetap: recording error: %v\n", recordErr)
			}
		}

		if readErr != nil {
			if readErr == io.EOF {
				// Flush any remaining buffered data
				if flushErr := r.Flush(source); flushErr != nil {
					fmt.Fprintf(os.Stderr, "ioetap: flush error: %v\n", flushErr)
				}
				return nil
			}
			return fmt.Errorf("read error: %w", readErr)
		}
	}
}

// Close flushes and closes the recording file.
func (r *Recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.writer.Flush(); err != nil {
		r.file.Close()
		return fmt.Errorf("failed to flush recording: %w", err)
	}

	return r.file.Close()
}
