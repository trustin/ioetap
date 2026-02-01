package recorder

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestRecorder_SequenceNumbers(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "test.jsonl")

	rec, err := NewRecorder(filename, 0)
	if err != nil {
		t.Fatalf("failed to create recorder: %v", err)
	}

	// Record multiple entries (with newlines for complete lines)
	for i := 0; i < 5; i++ {
		if err := rec.Record(Stdout, []byte("test\n")); err != nil {
			t.Fatalf("failed to record: %v", err)
		}
	}

	if err := rec.Close(); err != nil {
		t.Fatalf("failed to close recorder: %v", err)
	}

	// Read and verify sequence numbers
	file, err := os.Open(filename)
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	expectedSeq := uint64(0)
	for scanner.Scan() {
		var record Record
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatalf("failed to parse record: %v", err)
		}
		if record.Seq != expectedSeq {
			t.Errorf("expected seq %d, got %d", expectedSeq, record.Seq)
		}
		expectedSeq++
	}

	if expectedSeq != 5 {
		t.Errorf("expected 5 records, got %d", expectedSeq)
	}
}

func TestRecorder_ConcurrentRecording(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "test.jsonl")

	rec, err := NewRecorder(filename, 0)
	if err != nil {
		t.Fatalf("failed to create recorder: %v", err)
	}

	// Record from multiple goroutines concurrently (with newlines)
	var wg sync.WaitGroup
	numGoroutines := 10
	recordsPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < recordsPerGoroutine; j++ {
				if err := rec.Record(Stdout, []byte("test\n")); err != nil {
					t.Errorf("goroutine %d: failed to record: %v", id, err)
				}
			}
		}(i)
	}

	wg.Wait()

	if err := rec.Close(); err != nil {
		t.Fatalf("failed to close recorder: %v", err)
	}

	// Read and verify all records
	file, err := os.Open(filename)
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	seqNumbers := make(map[uint64]bool)
	recordCount := 0

	for scanner.Scan() {
		var record Record
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatalf("failed to parse record: %v", err)
		}

		if seqNumbers[record.Seq] {
			t.Errorf("duplicate sequence number: %d", record.Seq)
		}
		seqNumbers[record.Seq] = true
		recordCount++
	}

	expectedCount := numGoroutines * recordsPerGoroutine
	if recordCount != expectedCount {
		t.Errorf("expected %d records, got %d", expectedCount, recordCount)
	}

	// Verify sequence numbers are 0 to N-1
	for i := uint64(0); i < uint64(expectedCount); i++ {
		if !seqNumbers[i] {
			t.Errorf("missing sequence number: %d", i)
		}
	}
}

func TestRecorder_ValidNDJSON(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "test.jsonl")

	rec, err := NewRecorder(filename, 0)
	if err != nil {
		t.Fatalf("failed to create recorder: %v", err)
	}

	// Record entries with different sources (with newlines)
	sources := []Source{Stdin, Stdout, Stderr}
	sourceNames := []string{"stdin", "stdout", "stderr"}
	for _, source := range sources {
		if err := rec.Record(source, []byte("test data\n")); err != nil {
			t.Fatalf("failed to record: %v", err)
		}
	}

	if err := rec.Close(); err != nil {
		t.Fatalf("failed to close recorder: %v", err)
	}

	// Read and verify each line is valid JSON
	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	lines := bytes.Split(bytes.TrimSpace(content), []byte("\n"))
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}

	for i, line := range lines {
		var record Record
		if err := json.Unmarshal(line, &record); err != nil {
			t.Errorf("line %d is not valid JSON: %v", i, err)
		}
		if record.Source != sourceNames[i] {
			t.Errorf("line %d: expected source %s, got %s", i, sourceNames[i], record.Source)
		}
	}
}

func TestRecorder_CopyAndRecord(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "test.jsonl")

	rec, err := NewRecorder(filename, 0)
	if err != nil {
		t.Fatalf("failed to create recorder: %v", err)
	}

	// Create input and output buffers
	input := bytes.NewBufferString("Hello, World!")
	output := &bytes.Buffer{}

	// Copy and record
	if err := rec.CopyAndRecord(Stdout, input, output); err != nil {
		t.Fatalf("CopyAndRecord failed: %v", err)
	}

	if err := rec.Close(); err != nil {
		t.Fatalf("failed to close recorder: %v", err)
	}

	// Verify output received the data
	if output.String() != "Hello, World!" {
		t.Errorf("expected output 'Hello, World!', got %s", output.String())
	}

	// Verify recording
	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	var record Record
	if err := json.Unmarshal(bytes.TrimSpace(content), &record); err != nil {
		t.Fatalf("failed to parse record: %v", err)
	}

	if record.Source != "stdout" {
		t.Errorf("expected source stdout, got %s", record.Source)
	}
	if record.Content != "Hello, World!" {
		t.Errorf("expected content 'Hello, World!', got %s", record.Content)
	}
}

func TestRecorder_FileCreationError(t *testing.T) {
	// Try to create a recorder in a non-existent directory
	_, err := NewRecorder("/nonexistent/directory/test.jsonl", 0)
	if err == nil {
		t.Error("expected error for non-existent directory, got nil")
	}
}

func TestRecorder_LineBuffering(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "test.jsonl")

	rec, err := NewRecorder(filename, 0)
	if err != nil {
		t.Fatalf("failed to create recorder: %v", err)
	}

	// Simulate fragmented input: "hello\n" split into "he" and "llo\n"
	if err := rec.Record(Stdout, []byte("he")); err != nil {
		t.Fatalf("failed to record: %v", err)
	}
	if err := rec.Record(Stdout, []byte("llo\n")); err != nil {
		t.Fatalf("failed to record: %v", err)
	}

	if err := rec.Close(); err != nil {
		t.Fatalf("failed to close recorder: %v", err)
	}

	// Should result in exactly one record with "hello"
	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	lines := bytes.Split(bytes.TrimSpace(content), []byte("\n"))
	if len(lines) != 1 {
		t.Fatalf("expected 1 record, got %d", len(lines))
	}

	var record Record
	if err := json.Unmarshal(lines[0], &record); err != nil {
		t.Fatalf("failed to parse record: %v", err)
	}

	if record.Content != "hello" {
		t.Errorf("expected content 'hello', got %v", record.Content)
	}
	if record.End != "\n" {
		t.Errorf("expected end '\\n', got %q", record.End)
	}
}

func TestRecorder_LineBufferingMultipleLines(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "test.jsonl")

	rec, err := NewRecorder(filename, 0)
	if err != nil {
		t.Fatalf("failed to create recorder: %v", err)
	}

	// Simulate fragmented input: "line1\nline2\nline3" split across chunks
	if err := rec.Record(Stdout, []byte("li")); err != nil {
		t.Fatalf("failed to record: %v", err)
	}
	if err := rec.Record(Stdout, []byte("ne1\nli")); err != nil {
		t.Fatalf("failed to record: %v", err)
	}
	if err := rec.Record(Stdout, []byte("ne2\nline3")); err != nil {
		t.Fatalf("failed to record: %v", err)
	}

	// Flush remaining buffer
	if err := rec.Flush(Stdout); err != nil {
		t.Fatalf("failed to flush: %v", err)
	}

	if err := rec.Close(); err != nil {
		t.Fatalf("failed to close recorder: %v", err)
	}

	// Should result in 3 records
	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	lines := bytes.Split(bytes.TrimSpace(content), []byte("\n"))
	if len(lines) != 3 {
		t.Fatalf("expected 3 records, got %d", len(lines))
	}

	expected := []struct {
		content string
		end     string
	}{
		{"line1", "\n"},
		{"line2", "\n"},
		{"line3", ""},
	}

	for i, line := range lines {
		var record Record
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatalf("failed to parse record %d: %v", i, err)
		}

		if record.Content != expected[i].content {
			t.Errorf("record %d: expected content %q, got %v", i, expected[i].content, record.Content)
		}
		if record.End != expected[i].end {
			t.Errorf("record %d: expected end %q, got %q", i, expected[i].end, record.End)
		}
	}
}

func TestRecorder_FlushWithoutData(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "test.jsonl")

	rec, err := NewRecorder(filename, 0)
	if err != nil {
		t.Fatalf("failed to create recorder: %v", err)
	}

	// Flush without any data should not cause error
	if err := rec.Flush(Stdout); err != nil {
		t.Errorf("Flush without data should not error: %v", err)
	}

	if err := rec.Close(); err != nil {
		t.Fatalf("failed to close recorder: %v", err)
	}

	// File should be empty
	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	if len(content) != 0 {
		t.Errorf("expected empty file, got %q", content)
	}
}

func TestRecorder_CopyAndRecordFlushesAtEOF(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "test.jsonl")

	rec, err := NewRecorder(filename, 0)
	if err != nil {
		t.Fatalf("failed to create recorder: %v", err)
	}

	// Use a custom reader that returns data in small chunks
	chunks := []string{"hel", "lo"}
	reader := &chunkedReader{chunks: chunks}
	output := &bytes.Buffer{}

	if err := rec.CopyAndRecord(Stdout, reader, output); err != nil {
		t.Fatalf("CopyAndRecord failed: %v", err)
	}

	if err := rec.Close(); err != nil {
		t.Fatalf("failed to close recorder: %v", err)
	}

	// Verify output
	if output.String() != "hello" {
		t.Errorf("expected output 'hello', got %s", output.String())
	}

	// Verify recording - should be one record (flushed at EOF)
	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	lines := bytes.Split(bytes.TrimSpace(content), []byte("\n"))
	if len(lines) != 1 {
		t.Fatalf("expected 1 record, got %d", len(lines))
	}

	var record Record
	if err := json.Unmarshal(lines[0], &record); err != nil {
		t.Fatalf("failed to parse record: %v", err)
	}

	if record.Content != "hello" {
		t.Errorf("expected content 'hello', got %v", record.Content)
	}
	// No newline at end since input didn't have one
	if record.End != "" {
		t.Errorf("expected empty end, got %q", record.End)
	}
}

// chunkedReader returns data in predefined chunks
type chunkedReader struct {
	chunks []string
	idx    int
}

func (r *chunkedReader) Read(p []byte) (n int, err error) {
	if r.idx >= len(r.chunks) {
		return 0, io.EOF
	}
	chunk := r.chunks[r.idx]
	r.idx++
	n = copy(p, chunk)
	return n, nil
}

func TestRecorder_TruncationSingleChunk(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "test.jsonl")

	// Create recorder with max line length of 10
	rec, err := NewRecorder(filename, 10)
	if err != nil {
		t.Fatalf("failed to create recorder: %v", err)
	}

	// Record a line longer than 10 bytes
	if err := rec.Record(Stdout, []byte("this is a very long line\n")); err != nil {
		t.Fatalf("failed to record: %v", err)
	}

	if err := rec.Close(); err != nil {
		t.Fatalf("failed to close recorder: %v", err)
	}

	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	var record Record
	if err := json.Unmarshal(bytes.TrimSpace(content), &record); err != nil {
		t.Fatalf("failed to parse record: %v", err)
	}

	// Content should be truncated to 10 bytes
	if record.Content != "this is a " {
		t.Errorf("expected truncated content 'this is a ', got %q", record.Content)
	}
	if !record.Truncated {
		t.Error("expected Truncated to be true")
	}
	if record.End != "\n" {
		t.Errorf("expected end '\\n', got %q", record.End)
	}
}

func TestRecorder_TruncationMultipleChunks(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "test.jsonl")

	// Create recorder with max line length of 10
	rec, err := NewRecorder(filename, 10)
	if err != nil {
		t.Fatalf("failed to create recorder: %v", err)
	}

	// Record in multiple chunks that together exceed 10 bytes
	if err := rec.Record(Stdout, []byte("abcde")); err != nil {
		t.Fatalf("failed to record: %v", err)
	}
	if err := rec.Record(Stdout, []byte("fghij")); err != nil {
		t.Fatalf("failed to record: %v", err)
	}
	// This chunk should be skipped (we're already at limit)
	if err := rec.Record(Stdout, []byte("klmno")); err != nil {
		t.Fatalf("failed to record: %v", err)
	}
	// Newline triggers write
	if err := rec.Record(Stdout, []byte("\n")); err != nil {
		t.Fatalf("failed to record: %v", err)
	}

	if err := rec.Close(); err != nil {
		t.Fatalf("failed to close recorder: %v", err)
	}

	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	var record Record
	if err := json.Unmarshal(bytes.TrimSpace(content), &record); err != nil {
		t.Fatalf("failed to parse record: %v", err)
	}

	// Content should be exactly 10 bytes (truncated at limit)
	if record.Content != "abcdefghij" {
		t.Errorf("expected truncated content 'abcdefghij', got %q", record.Content)
	}
	if !record.Truncated {
		t.Error("expected Truncated to be true")
	}
}

func TestRecorder_TruncationExactLimit(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "test.jsonl")

	// Create recorder with max line length of 11 (10 content + 1 newline)
	rec, err := NewRecorder(filename, 11)
	if err != nil {
		t.Fatalf("failed to create recorder: %v", err)
	}

	// Record exactly 11 bytes including newline (should NOT be truncated)
	if err := rec.Record(Stdout, []byte("abcdefghij\n")); err != nil {
		t.Fatalf("failed to record: %v", err)
	}

	if err := rec.Close(); err != nil {
		t.Fatalf("failed to close recorder: %v", err)
	}

	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	var record Record
	if err := json.Unmarshal(bytes.TrimSpace(content), &record); err != nil {
		t.Fatalf("failed to parse record: %v", err)
	}

	// Content should be exactly 10 bytes (newline is in End field)
	if record.Content != "abcdefghij" {
		t.Errorf("expected content 'abcdefghij', got %q", record.Content)
	}
	if record.Truncated {
		t.Error("expected Truncated to be false for exact limit")
	}
}

func TestRecorder_TruncationUnlimited(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "test.jsonl")

	// Create recorder with unlimited line length
	rec, err := NewRecorder(filename, 0)
	if err != nil {
		t.Fatalf("failed to create recorder: %v", err)
	}

	// Record a very long line
	longLine := strings.Repeat("x", 10000) + "\n"
	if err := rec.Record(Stdout, []byte(longLine)); err != nil {
		t.Fatalf("failed to record: %v", err)
	}

	if err := rec.Close(); err != nil {
		t.Fatalf("failed to close recorder: %v", err)
	}

	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	var record Record
	if err := json.Unmarshal(bytes.TrimSpace(content), &record); err != nil {
		t.Fatalf("failed to parse record: %v", err)
	}

	// Content should be the full 10000 characters
	contentStr := record.Content.(string)
	if len(contentStr) != 10000 {
		t.Errorf("expected content length 10000, got %d", len(contentStr))
	}
	if record.Truncated {
		t.Error("expected Truncated to be false for unlimited mode")
	}
}

func TestRecorder_TruncationFlushAtEOF(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "test.jsonl")

	// Create recorder with max line length of 10
	rec, err := NewRecorder(filename, 10)
	if err != nil {
		t.Fatalf("failed to create recorder: %v", err)
	}

	// Record data that exceeds limit but has no newline
	if err := rec.Record(Stdout, []byte("this is a very long line without newline")); err != nil {
		t.Fatalf("failed to record: %v", err)
	}

	// Flush (simulating EOF)
	if err := rec.Flush(Stdout); err != nil {
		t.Fatalf("failed to flush: %v", err)
	}

	if err := rec.Close(); err != nil {
		t.Fatalf("failed to close recorder: %v", err)
	}

	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	var record Record
	if err := json.Unmarshal(bytes.TrimSpace(content), &record); err != nil {
		t.Fatalf("failed to parse record: %v", err)
	}

	// Content should be truncated to 10 bytes
	if record.Content != "this is a " {
		t.Errorf("expected truncated content 'this is a ', got %q", record.Content)
	}
	if !record.Truncated {
		t.Error("expected Truncated to be true")
	}
	// No newline since input didn't have one
	if record.End != "" {
		t.Errorf("expected empty end, got %q", record.End)
	}
}

func TestRecorder_TruncationCRLF(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "test.jsonl")

	// Create recorder with max line length of 10
	rec, err := NewRecorder(filename, 10)
	if err != nil {
		t.Fatalf("failed to create recorder: %v", err)
	}

	// Record a line longer than 10 bytes with CRLF ending
	if err := rec.Record(Stdout, []byte("this is a very long line\r\n")); err != nil {
		t.Fatalf("failed to record: %v", err)
	}

	if err := rec.Close(); err != nil {
		t.Fatalf("failed to close recorder: %v", err)
	}

	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	var record Record
	if err := json.Unmarshal(bytes.TrimSpace(content), &record); err != nil {
		t.Fatalf("failed to parse record: %v", err)
	}

	// Content should be truncated
	if record.Content != "this is a " {
		t.Errorf("expected truncated content 'this is a ', got %q", record.Content)
	}
	if !record.Truncated {
		t.Error("expected Truncated to be true")
	}
	// Line ending should be preserved
	if record.End != "\r\n" {
		t.Errorf("expected end '\\r\\n', got %q", record.End)
	}
}

func TestRecorder_TruncationMultipleLines(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "test.jsonl")

	// Create recorder with max line length of 10
	rec, err := NewRecorder(filename, 10)
	if err != nil {
		t.Fatalf("failed to create recorder: %v", err)
	}

	// Record multiple lines, some truncated
	if err := rec.Record(Stdout, []byte("short\nthis is a very long line\nok\n")); err != nil {
		t.Fatalf("failed to record: %v", err)
	}

	if err := rec.Close(); err != nil {
		t.Fatalf("failed to close recorder: %v", err)
	}

	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	lines := bytes.Split(bytes.TrimSpace(content), []byte("\n"))
	if len(lines) != 3 {
		t.Fatalf("expected 3 records, got %d", len(lines))
	}

	// First line: short (not truncated)
	var record1 Record
	if err := json.Unmarshal(lines[0], &record1); err != nil {
		t.Fatalf("failed to parse record 1: %v", err)
	}
	if record1.Content != "short" {
		t.Errorf("record 1: expected content 'short', got %q", record1.Content)
	}
	if record1.Truncated {
		t.Error("record 1: expected Truncated to be false")
	}

	// Second line: truncated
	var record2 Record
	if err := json.Unmarshal(lines[1], &record2); err != nil {
		t.Fatalf("failed to parse record 2: %v", err)
	}
	if record2.Content != "this is a " {
		t.Errorf("record 2: expected content 'this is a ', got %q", record2.Content)
	}
	if !record2.Truncated {
		t.Error("record 2: expected Truncated to be true")
	}

	// Third line: ok (not truncated)
	var record3 Record
	if err := json.Unmarshal(lines[2], &record3); err != nil {
		t.Fatalf("failed to parse record 3: %v", err)
	}
	if record3.Content != "ok" {
		t.Errorf("record 3: expected content 'ok', got %q", record3.Content)
	}
	if record3.Truncated {
		t.Error("record 3: expected Truncated to be false")
	}
}

func TestRecorder_TruncationJSONContent(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "test.jsonl")

	// Create recorder with max line length of 20
	rec, err := NewRecorder(filename, 20)
	if err != nil {
		t.Fatalf("failed to create recorder: %v", err)
	}

	// Record a large JSON object that exceeds the limit
	if err := rec.Record(Stdout, []byte(`{"key":"this is a very long value that exceeds the limit"}`+"\n")); err != nil {
		t.Fatalf("failed to record: %v", err)
	}

	if err := rec.Close(); err != nil {
		t.Fatalf("failed to close recorder: %v", err)
	}

	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	var record Record
	if err := json.Unmarshal(bytes.TrimSpace(content), &record); err != nil {
		t.Fatalf("failed to parse record: %v", err)
	}

	// Content should be truncated (not valid JSON anymore, so falls back to text)
	if record.Encoding != "text" {
		t.Errorf("expected encoding 'text', got %q", record.Encoding)
	}
	if !record.Truncated {
		t.Error("expected Truncated to be true")
	}
	// Content should be first 20 bytes
	contentStr := record.Content.(string)
	if len(contentStr) != 20 {
		t.Errorf("expected content length 20, got %d", len(contentStr))
	}
}
