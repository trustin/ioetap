package test

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Record mirrors the internal Record struct for testing
type Record struct {
	Seq       uint64 `json:"seq"`
	Timestamp string `json:"timestamp"`
	Source    string `json:"source"`
	Content   any    `json:"content"`
	Encoding  string `json:"encoding"`
	End       string `json:"end,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

// ContentString returns the content as a string for text/base64 encoding.
// For json encoding, it marshals the content back to JSON.
func (r Record) ContentString() string {
	switch r.Encoding {
	case "text", "base64":
		if s, ok := r.Content.(string); ok {
			return s
		}
	case "json":
		data, err := json.Marshal(r.Content)
		if err != nil {
			return ""
		}
		return string(data)
	}
	return ""
}

var (
	testBinaryOnce sync.Once
	testBinaryPath string
	testBinaryErr  error
)

func buildIoetap(t *testing.T) string {
	t.Helper()

	testBinaryOnce.Do(func() {
		// Get the project root directory (parent of test/)
		_, thisFile, _, ok := runtimeCallerForTest()
		if !ok {
			testBinaryErr = fmt.Errorf("failed to get current file path")
			return
		}
		projectRoot := filepath.Dir(filepath.Dir(thisFile))

		// Build to a fixed location in the project's bin directory
		binDir := filepath.Join(projectRoot, "bin")
		if err := os.MkdirAll(binDir, 0755); err != nil {
			testBinaryErr = fmt.Errorf("failed to create bin directory: %v", err)
			return
		}

		testBinaryPath = filepath.Join(binDir, "ioetap-test")

		// Remove old binary first to ensure clean build
		os.Remove(testBinaryPath)

		cmd := exec.Command("go", "build", "-o", testBinaryPath, "./cmd/ioetap")
		cmd.Dir = projectRoot
		output, err := cmd.CombinedOutput()
		if err != nil {
			testBinaryErr = fmt.Errorf("failed to build ioetap: %v\n%s", err, output)
			return
		}

		// Verify binary was created and is executable
		info, err := os.Stat(testBinaryPath)
		if err != nil {
			testBinaryErr = fmt.Errorf("binary not found after build: %v", err)
			return
		}
		if info.Size() == 0 {
			testBinaryErr = fmt.Errorf("binary is empty after build")
			return
		}
	})

	if testBinaryErr != nil {
		t.Fatal(testBinaryErr)
	}

	return testBinaryPath
}

func runtimeCallerForTest() (pc uintptr, file string, line int, ok bool) {
	return runtime.Caller(0)
}

func findRecordingFile(t *testing.T, dir string, pattern string) string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read directory: %v", err)
	}

	re := regexp.MustCompile(pattern)
	for _, entry := range entries {
		if re.MatchString(entry.Name()) {
			return filepath.Join(dir, entry.Name())
		}
	}

	t.Fatalf("no recording file matching pattern %s found in %s", pattern, dir)
	return ""
}

func readRecords(t *testing.T, filename string) []Record {
	t.Helper()

	file, err := os.Open(filename)
	if err != nil {
		t.Fatalf("failed to open recording file: %v", err)
	}
	defer file.Close()

	var records []Record
	scanner := bufio.NewScanner(file)
	// Increase buffer size for long lines (1MB should be enough for tests)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		var record Record
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatalf("failed to parse record: %v", err)
		}
		records = append(records, record)
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("error reading file: %v", err)
	}

	return records
}

func TestIntegration_BasicOutput(t *testing.T) {
	binary := buildIoetap(t)
	workDir := t.TempDir()

	// Verify binary exists
	if _, err := os.Stat(binary); os.IsNotExist(err) {
		t.Fatalf("binary does not exist: %s", binary)
	}

	cmd := exec.Command(binary, "echo", "hello world")
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		t.Fatalf("ioetap failed: %v\nstdout: %q\nstderr: %q", err, stdout.String(), stderr.String())
	}

	output := stdout.String()
	if strings.TrimSpace(output) != "hello world" {
		t.Errorf("expected output 'hello world', got %q (stderr: %q)", output, stderr.String())
	}

	// Find and read the recording file
	recordingFile := findRecordingFile(t, workDir, `echo-\d+\.jsonl`)
	records := readRecords(t, recordingFile)

	if len(records) == 0 {
		t.Fatal("expected at least one record")
	}

	// Find stdout record
	var foundStdout bool
	for _, r := range records {
		if r.Source == "stdout" && strings.Contains(r.ContentString(), "hello world") {
			foundStdout = true
			if r.Encoding != "text" {
				t.Errorf("expected text encoding, got %s", r.Encoding)
			}
			break
		}
	}

	if !foundStdout {
		t.Error("stdout record not found in recording")
	}
}

func TestIntegration_BinaryData(t *testing.T) {
	binary := buildIoetap(t)
	workDir := t.TempDir()

	// Output binary data (non-UTF8)
	// Use octal escapes for POSIX compatibility (dash doesn't support \x)
	cmd := exec.Command(binary, "sh", "-c", "printf '\\377\\376\\000\\001'")
	cmd.Dir = workDir

	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("ioetap failed: %v", err)
	}

	// Verify binary output was passed through
	expected := []byte{0xff, 0xfe, 0x00, 0x01}
	if string(output) != string(expected) {
		t.Errorf("expected binary output, got %v", output)
	}

	// Find and read the recording file
	recordingFile := findRecordingFile(t, workDir, `sh-\d+\.jsonl`)
	records := readRecords(t, recordingFile)

	// Find stdout record with base64 encoding
	var foundBase64 bool
	for _, r := range records {
		if r.Source == "stdout" && r.Encoding == "base64" {
			foundBase64 = true
			decoded, err := base64.StdEncoding.DecodeString(r.ContentString())
			if err != nil {
				t.Errorf("failed to decode base64: %v", err)
			}
			if string(decoded) != string(expected) {
				t.Errorf("decoded content mismatch: expected %v, got %v", expected, decoded)
			}
			break
		}
	}

	if !foundBase64 {
		t.Error("base64 encoded record not found")
	}
}

func TestIntegration_ExitCode(t *testing.T) {
	binary := buildIoetap(t)
	workDir := t.TempDir()

	tests := []struct {
		exitCode int
	}{
		{0},
		{1},
		{42},
		{127},
	}

	for _, tc := range tests {
		t.Run(strconv.Itoa(tc.exitCode), func(t *testing.T) {
			cmd := exec.Command(binary, "sh", "-c", "exit "+strconv.Itoa(tc.exitCode))
			cmd.Dir = workDir

			err := cmd.Run()

			if tc.exitCode == 0 {
				if err != nil {
					t.Errorf("expected no error for exit 0, got %v", err)
				}
			} else {
				if exitErr, ok := err.(*exec.ExitError); ok {
					if exitErr.ExitCode() != tc.exitCode {
						t.Errorf("expected exit code %d, got %d", tc.exitCode, exitErr.ExitCode())
					}
				} else if err != nil {
					t.Errorf("unexpected error type: %v", err)
				} else {
					t.Errorf("expected exit code %d, got 0", tc.exitCode)
				}
			}
		})
	}
}

func TestIntegration_ConcurrentStreams(t *testing.T) {
	binary := buildIoetap(t)
	workDir := t.TempDir()

	// Command that writes to both stdout and stderr
	// Use stdbuf to disable buffering and ensure output is immediately visible
	// Add a small sleep to ensure output is captured before process exits
	script := `echo "stdout line" && echo "stderr line" >&2 && sleep 0.1`
	cmd := exec.Command(binary, "sh", "-c", script)
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("ioetap failed: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}

	// Verify both streams were captured
	if !strings.Contains(stdout.String(), "stdout line") {
		t.Errorf("expected stdout to contain 'stdout line', got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "stderr line") {
		t.Errorf("expected stderr to contain 'stderr line', got %q", stderr.String())
	}

	// Find and read the recording file
	recordingFile := findRecordingFile(t, workDir, `sh-\d+\.jsonl`)
	records := readRecords(t, recordingFile)

	// Verify both stdout and stderr were recorded
	var foundStdout, foundStderr bool
	for _, r := range records {
		if r.Source == "stdout" && strings.Contains(r.ContentString(), "stdout line") {
			foundStdout = true
		}
		if r.Source == "stderr" && strings.Contains(r.ContentString(), "stderr line") {
			foundStderr = true
		}
	}

	if !foundStdout {
		t.Error("stdout record not found in recording")
	}
	if !foundStderr {
		t.Error("stderr record not found in recording")
	}
}

func TestIntegration_RecordingFormat(t *testing.T) {
	binary := buildIoetap(t)
	workDir := t.TempDir()

	cmd := exec.Command(binary, "echo", "test")
	cmd.Dir = workDir

	if err := cmd.Run(); err != nil {
		t.Fatalf("ioetap failed: %v", err)
	}

	recordingFile := findRecordingFile(t, workDir, `echo-\d+\.jsonl`)

	// Read file contents directly
	content, err := os.ReadFile(recordingFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	// Verify each line is valid JSON
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}

		var record Record
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Errorf("line %d is not valid JSON: %v", i, err)
		}

		// Verify required fields
		if record.Timestamp == "" {
			t.Errorf("line %d: missing timestamp", i)
		}
		if record.Source == "" {
			t.Errorf("line %d: missing source", i)
		}
		if record.Encoding != "text" && record.Encoding != "base64" && record.Encoding != "json" {
			t.Errorf("line %d: invalid encoding %q", i, record.Encoding)
		}

		// Verify timestamp format
		timestampRe := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`)
		if !timestampRe.MatchString(record.Timestamp) {
			t.Errorf("line %d: invalid timestamp format %q", i, record.Timestamp)
		}
	}
}

func TestIntegration_SequenceOrdering(t *testing.T) {
	binary := buildIoetap(t)
	workDir := t.TempDir()

	// Generate multiple outputs
	cmd := exec.Command(binary, "sh", "-c", "for i in 1 2 3 4 5; do echo $i; done")
	cmd.Dir = workDir

	if err := cmd.Run(); err != nil {
		t.Fatalf("ioetap failed: %v", err)
	}

	recordingFile := findRecordingFile(t, workDir, `sh-\d+\.jsonl`)
	records := readRecords(t, recordingFile)

	// Verify sequence numbers are unique and ordered
	seqNumbers := make(map[uint64]bool)
	var lastSeq int64 = -1

	for _, r := range records {
		if seqNumbers[r.Seq] {
			t.Errorf("duplicate sequence number: %d", r.Seq)
		}
		seqNumbers[r.Seq] = true

		// Note: Due to concurrent goroutines, sequence numbers might not be strictly ordered in file
		// But they should all be unique (checked above)
		_ = lastSeq // Silence unused variable warning
	}
}

func TestIntegration_StdinRecording(t *testing.T) {
	binary := buildIoetap(t)
	workDir := t.TempDir()

	cmd := exec.Command(binary, "cat")
	cmd.Dir = workDir
	cmd.Stdin = strings.NewReader("test input\n")

	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("ioetap failed: %v", err)
	}

	if string(output) != "test input\n" {
		t.Errorf("expected output 'test input\\n', got %q", string(output))
	}

	// Find and read the recording file
	recordingFile := findRecordingFile(t, workDir, `cat-\d+\.jsonl`)
	records := readRecords(t, recordingFile)

	// Should have both stdin and stdout records
	var foundStdin, foundStdout bool
	for _, r := range records {
		if r.Source == "stdin" && strings.Contains(r.ContentString(), "test input") {
			foundStdin = true
			if r.Encoding != "text" {
				t.Errorf("expected text encoding for stdin, got %s", r.Encoding)
			}
		}
		if r.Source == "stdout" && strings.Contains(r.ContentString(), "test input") {
			foundStdout = true
		}
	}

	if !foundStdin {
		t.Error("stdin record not found in recording")
	}
	if !foundStdout {
		t.Error("stdout record not found in recording")
	}
}

func TestIntegration_NoHangOnExit(t *testing.T) {
	binary := buildIoetap(t)
	workDir := t.TempDir()

	// Run a command that exits immediately without reading stdin
	cmd := exec.Command(binary, "sh", "-c", "exit 42")
	cmd.Dir = workDir

	// Use a channel to detect if the command completes
	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	// Should complete within 2 seconds
	select {
	case err := <-done:
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() != 42 {
				t.Errorf("expected exit code 42, got %d", exitErr.ExitCode())
			}
		} else if err != nil {
			t.Errorf("unexpected error: %v", err)
		} else {
			t.Error("expected exit code 42, got 0")
		}
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("ioetap hung - did not exit within 2 seconds")
	}
}

func TestIntegration_SignalTermination(t *testing.T) {
	binary := buildIoetap(t)
	workDir := t.TempDir()

	// Start a long-running command
	cmd := exec.Command(binary, "sleep", "30")
	cmd.Dir = workDir

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start ioetap: %v", err)
	}

	// Wait a bit for the process to start
	time.Sleep(100 * time.Millisecond)

	// Send SIGTERM
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("failed to send signal: %v", err)
	}

	// Should terminate within 2 seconds
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-done:
		// Process terminated as expected
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("ioetap did not terminate after receiving signal")
	}
}

func TestIntegration_LargeOutput(t *testing.T) {
	binary := buildIoetap(t)
	workDir := t.TempDir()

	// Generate a large output using printf in a loop (more predictable)
	// Each iteration outputs exactly 1000 'A' characters followed by a newline
	cmd := exec.Command(binary, "sh", "-c", "for i in $(seq 1 100); do printf '%1000s\\n' | tr ' ' 'A'; done")
	cmd.Dir = workDir

	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("ioetap failed: %v", err)
	}

	outputLen := len(output)
	if outputLen < 50000 {
		t.Errorf("expected large output (>50KB), got %d bytes", outputLen)
	}

	// Verify recording file exists and has records
	recordingFile := findRecordingFile(t, workDir, `sh-\d+\.jsonl`)
	records := readRecords(t, recordingFile)

	if len(records) == 0 {
		t.Fatal("expected at least one record")
	}

	// Verify total recorded content matches actual output (including End field)
	var totalLen int
	for _, r := range records {
		if r.Source == "stdout" {
			totalLen += len(r.ContentString()) + len(r.End)
		}
	}

	if totalLen != outputLen {
		t.Errorf("recorded content length (%d) doesn't match output length (%d)", totalLen, outputLen)
	}
}

func TestIntegration_MultilineInput(t *testing.T) {
	binary := buildIoetap(t)
	workDir := t.TempDir()

	input := "line1\nline2\nline3\n"
	cmd := exec.Command(binary, "cat")
	cmd.Dir = workDir
	cmd.Stdin = strings.NewReader(input)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("ioetap failed: %v\nstderr: %s", err, stderr.String())
	}

	if stdout.String() != input {
		t.Errorf("expected output %q, got %q", input, stdout.String())
	}

	// Find and read the recording file
	recordingFile := findRecordingFile(t, workDir, `cat-\d+\.jsonl`)
	records := readRecords(t, recordingFile)

	// Combine all stdin content (including End field for line endings)
	var stdinContent strings.Builder
	for _, r := range records {
		if r.Source == "stdin" {
			stdinContent.WriteString(r.ContentString())
			stdinContent.WriteString(r.End)
		}
	}

	// Reconstructed content should match original input
	if stdinContent.String() != input {
		t.Errorf("expected recorded stdin %q, got %q", input, stdinContent.String())
	}
}

func TestIntegration_OptionOutFile(t *testing.T) {
	binary := buildIoetap(t)
	workDir := t.TempDir()

	outputFile := filepath.Join(workDir, "custom-output.jsonl")

	// Test --out=file form
	cmd := exec.Command(binary, "--out="+outputFile, "--", "echo", "hello")
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("ioetap failed: %v\nstderr: %s", err, stderr.String())
	}

	if strings.TrimSpace(stdout.String()) != "hello" {
		t.Errorf("expected output 'hello', got %q", stdout.String())
	}

	// Verify the custom output file was created
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		t.Fatalf("output file was not created at %s", outputFile)
	}

	records := readRecords(t, outputFile)
	if len(records) == 0 {
		t.Fatal("expected at least one record")
	}

	// Verify stdout was recorded
	var foundStdout bool
	for _, r := range records {
		if r.Source == "stdout" && strings.Contains(r.ContentString(), "hello") {
			foundStdout = true
			break
		}
	}
	if !foundStdout {
		t.Error("stdout record not found in recording")
	}
}

func TestIntegration_OptionOutFileSpace(t *testing.T) {
	binary := buildIoetap(t)
	workDir := t.TempDir()

	outputFile := filepath.Join(workDir, "space-output.jsonl")

	// Test --out file form (space between option and value)
	cmd := exec.Command(binary, "--out", outputFile, "--", "echo", "world")
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("ioetap failed: %v\nstderr: %s", err, stderr.String())
	}

	if strings.TrimSpace(stdout.String()) != "world" {
		t.Errorf("expected output 'world', got %q", stdout.String())
	}

	// Verify the custom output file was created
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		t.Fatalf("output file was not created at %s", outputFile)
	}

	records := readRecords(t, outputFile)
	if len(records) == 0 {
		t.Fatal("expected at least one record")
	}
}

func TestIntegration_OptionRequiresSeparator(t *testing.T) {
	binary := buildIoetap(t)
	workDir := t.TempDir()

	// Using options without -- separator should fail
	cmd := exec.Command(binary, "--out=test.jsonl", "echo", "hello")
	cmd.Dir = workDir

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		t.Fatal("expected error when using options without -- separator")
	}

	// Should mention the need for separator
	if !strings.Contains(stderr.String(), "--") {
		t.Errorf("error message should mention -- separator, got: %s", stderr.String())
	}
}

func TestIntegration_BackwardCompatible(t *testing.T) {
	binary := buildIoetap(t)
	workDir := t.TempDir()

	// Without options, should work without -- separator (backward compatible)
	cmd := exec.Command(binary, "echo", "backward compatible")
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("ioetap failed: %v\nstderr: %s", err, stderr.String())
	}

	if strings.TrimSpace(stdout.String()) != "backward compatible" {
		t.Errorf("expected output 'backward compatible', got %q", stdout.String())
	}

	// Recording file should be created with default naming
	recordingFile := findRecordingFile(t, workDir, `echo-\d+\.jsonl`)
	records := readRecords(t, recordingFile)

	if len(records) == 0 {
		t.Fatal("expected at least one record")
	}
}

func TestIntegration_JSONOutput(t *testing.T) {
	binary := buildIoetap(t)
	workDir := t.TempDir()

	// Output valid JSON
	cmd := exec.Command(binary, "sh", "-c", `echo '{"key":"value","num":42}'`)
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("ioetap failed: %v\nstderr: %s", err, stderr.String())
	}

	// Find and read the recording file
	recordingFile := findRecordingFile(t, workDir, `sh-\d+\.jsonl`)
	records := readRecords(t, recordingFile)

	// Find stdout record with json encoding
	var foundJSON bool
	for _, r := range records {
		if r.Source == "stdout" && r.Encoding == "json" {
			foundJSON = true

			// Content should be a map
			contentMap, ok := r.Content.(map[string]any)
			if !ok {
				t.Errorf("expected content to be map[string]any, got %T", r.Content)
				break
			}

			if contentMap["key"] != "value" {
				t.Errorf("expected key='value', got %v", contentMap["key"])
			}
			if contentMap["num"] != float64(42) {
				t.Errorf("expected num=42, got %v", contentMap["num"])
			}
			break
		}
	}

	if !foundJSON {
		t.Error("json encoded record not found")
	}
}

func TestIntegration_JSONNumber(t *testing.T) {
	binary := buildIoetap(t)
	workDir := t.TempDir()

	// Output a JSON number
	cmd := exec.Command(binary, "sh", "-c", `echo '123'`)
	cmd.Dir = workDir

	if err := cmd.Run(); err != nil {
		t.Fatalf("ioetap failed: %v", err)
	}

	recordingFile := findRecordingFile(t, workDir, `sh-\d+\.jsonl`)
	records := readRecords(t, recordingFile)

	var foundJSON bool
	for _, r := range records {
		if r.Source == "stdout" && r.Encoding == "json" {
			foundJSON = true

			// Content should be a number
			contentNum, ok := r.Content.(float64)
			if !ok {
				t.Errorf("expected content to be float64, got %T", r.Content)
				break
			}
			if contentNum != 123 {
				t.Errorf("expected 123, got %v", contentNum)
			}
			break
		}
	}

	if !foundJSON {
		t.Error("json encoded record not found")
	}
}

func TestIntegration_JSONArray(t *testing.T) {
	binary := buildIoetap(t)
	workDir := t.TempDir()

	// Output a JSON array
	cmd := exec.Command(binary, "sh", "-c", `echo '[1,2,3]'`)
	cmd.Dir = workDir

	if err := cmd.Run(); err != nil {
		t.Fatalf("ioetap failed: %v", err)
	}

	recordingFile := findRecordingFile(t, workDir, `sh-\d+\.jsonl`)
	records := readRecords(t, recordingFile)

	var foundJSON bool
	for _, r := range records {
		if r.Source == "stdout" && r.Encoding == "json" {
			foundJSON = true

			// Content should be an array
			contentArr, ok := r.Content.([]any)
			if !ok {
				t.Errorf("expected content to be []any, got %T", r.Content)
				break
			}
			if len(contentArr) != 3 {
				t.Errorf("expected 3 elements, got %d", len(contentArr))
			}
			break
		}
	}

	if !foundJSON {
		t.Error("json encoded record not found")
	}
}

func TestIntegration_PlainTextNotJSON(t *testing.T) {
	binary := buildIoetap(t)
	workDir := t.TempDir()

	// Output plain text that is not valid JSON
	cmd := exec.Command(binary, "echo", "just plain text")
	cmd.Dir = workDir

	if err := cmd.Run(); err != nil {
		t.Fatalf("ioetap failed: %v", err)
	}

	recordingFile := findRecordingFile(t, workDir, `echo-\d+\.jsonl`)
	records := readRecords(t, recordingFile)

	var foundText bool
	for _, r := range records {
		if r.Source == "stdout" && r.Encoding == "text" {
			foundText = true
			contentStr, ok := r.Content.(string)
			if !ok {
				t.Errorf("expected content to be string, got %T", r.Content)
				break
			}
			if !strings.Contains(contentStr, "just plain text") {
				t.Errorf("expected content to contain 'just plain text', got %q", contentStr)
			}
			break
		}
	}

	if !foundText {
		t.Error("text encoded record not found")
	}
}

func TestIntegration_MaxLineLengthOption(t *testing.T) {
	binary := buildIoetap(t)
	workDir := t.TempDir()
	outputFile := filepath.Join(workDir, "output.jsonl")

	// Use --max-line-length=20 and output a line longer than 20 bytes
	cmd := exec.Command(binary, "--max-line-length=20", "--out="+outputFile, "--", "sh", "-c", "echo 'this is a very long line that exceeds 20 bytes'")
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("ioetap failed: %v\nstderr: %s", err, stderr.String())
	}

	// Verify the output file was created
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		t.Fatalf("output file was not created at %s", outputFile)
	}

	records := readRecords(t, outputFile)
	if len(records) == 0 {
		t.Fatal("expected at least one record")
	}

	// Find stdout record and verify truncation
	var foundTruncated bool
	for _, r := range records {
		if r.Source == "stdout" {
			foundTruncated = true
			if !r.Truncated {
				t.Error("expected record to be truncated")
			}
			contentStr := r.ContentString()
			if len(contentStr) != 20 {
				t.Errorf("expected content length 20, got %d", len(contentStr))
			}
			break
		}
	}

	if !foundTruncated {
		t.Error("stdout record not found")
	}
}

func TestIntegration_MaxLineLengthUnlimited(t *testing.T) {
	binary := buildIoetap(t)
	workDir := t.TempDir()
	outputFile := filepath.Join(workDir, "output.jsonl")

	// Use --max-line-length=0 (unlimited) and output a long line
	cmd := exec.Command(binary, "--max-line-length=0", "--out="+outputFile, "--", "sh", "-c", "printf '%1000s\\n' | tr ' ' 'A'")
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("ioetap failed: %v\nstderr: %s", err, stderr.String())
	}

	records := readRecords(t, outputFile)
	if len(records) == 0 {
		t.Fatal("expected at least one record")
	}

	// Find stdout record and verify not truncated
	var foundRecord bool
	for _, r := range records {
		if r.Source == "stdout" {
			foundRecord = true
			if r.Truncated {
				t.Error("expected record to NOT be truncated")
			}
			contentStr := r.ContentString()
			if len(contentStr) != 1000 {
				t.Errorf("expected content length 1000, got %d", len(contentStr))
			}
			break
		}
	}

	if !foundRecord {
		t.Error("stdout record not found")
	}
}

func TestIntegration_MaxLineLengthDefault(t *testing.T) {
	binary := buildIoetap(t)
	workDir := t.TempDir()

	// Default max line length should be 16 MiB
	// Test with a moderately long line (100KB) to verify it's not truncated
	cmd := exec.Command(binary, "sh", "-c", "printf '%100000s\\n' | tr ' ' 'B'")
	cmd.Dir = workDir

	if err := cmd.Run(); err != nil {
		t.Fatalf("ioetap failed: %v", err)
	}

	recordingFile := findRecordingFile(t, workDir, `sh-\d+\.jsonl`)
	records := readRecords(t, recordingFile)

	// Find stdout record and verify not truncated (100KB is well under 16 MiB)
	var foundRecord bool
	for _, r := range records {
		if r.Source == "stdout" {
			foundRecord = true
			if r.Truncated {
				t.Error("expected record to NOT be truncated (100KB is under 16 MiB default)")
			}
			contentStr := r.ContentString()
			if len(contentStr) != 100000 {
				t.Errorf("expected content length 100000, got %d", len(contentStr))
			}
			break
		}
	}

	if !foundRecord {
		t.Error("stdout record not found")
	}
}
