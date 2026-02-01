package recorder

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"time"
	"unicode/utf8"
)

// Record represents a single I/O record in the recording file.
type Record struct {
	Seq       uint64 `json:"seq"`       // Sequence number, starts from 0
	Timestamp string `json:"timestamp"` // UTC timestamp with ms precision
	Source    string `json:"source"`    // "stdin", "stdout", or "stderr"
	Content   any    `json:"-"`         // Content value (varies by encoding)
	Encoding  string `json:"encoding"`  // "text", "base64", or "json"
	End       string `json:"-"`         // Trailing CR/LF for text encoding (omitted if empty)
	Truncated bool   `json:"-"`         // true if line was truncated due to max length
}

const timestampFormat = "2006-01-02T15:04:05.000Z"

// NewRecord creates a new Record with automatic encoding detection.
// Priority: JSON > text > base64
// For text content, trailing CR/LF is extracted into the End field.
func NewRecord(seq uint64, timestamp time.Time, source string, data []byte) Record {
	// Try JSON first (trim whitespace for lenient parsing)
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && json.Valid(trimmed) {
		// json.Valid ensures ENTIRE content is valid JSON (no trailing data)
		// This rejects: {"a":1}blah, {"a":1}{"b":2}, etc.
		var parsed any
		if err := json.Unmarshal(trimmed, &parsed); err == nil {
			return Record{
				Seq:       seq,
				Timestamp: timestamp.UTC().Format(timestampFormat),
				Source:    source,
				Content:   parsed,
				Encoding:  "json",
			}
		}
	}

	// Then UTF-8 text (extract trailing CR/LF)
	if utf8.Valid(data) {
		content, trailing := splitTrailingCRLF(data)
		return Record{
			Seq:       seq,
			Timestamp: timestamp.UTC().Format(timestampFormat),
			Source:    source,
			Content:   string(content),
			Encoding:  "text",
			End:       string(trailing),
		}
	}

	// Finally base64
	return Record{
		Seq:       seq,
		Timestamp: timestamp.UTC().Format(timestampFormat),
		Source:    source,
		Content:   base64.StdEncoding.EncodeToString(data),
		Encoding:  "base64",
	}
}

// Line represents a single line of text with its line ending.
type Line struct {
	Content []byte
	End     []byte
}

// SplitLines splits data into lines, preserving line endings.
// Each line includes its terminating \n or \r\n in the End field.
// The last line may have an empty End if it doesn't end with a newline.
func SplitLines(data []byte) []Line {
	if len(data) == 0 {
		return []Line{{Content: nil, End: nil}}
	}

	var lines []Line
	start := 0

	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			// Check for \r\n
			var lineEnd, endStart int
			if i > start && data[i-1] == '\r' {
				lineEnd = i - 1
				endStart = i - 1
			} else {
				lineEnd = i
				endStart = i
			}

			lines = append(lines, Line{
				Content: data[start:lineEnd],
				End:     data[endStart : i+1],
			})
			start = i + 1
		}
	}

	// Handle remaining content after last newline (or entire content if no newline)
	if start < len(data) {
		content, trailing := splitTrailingCRLF(data[start:])
		lines = append(lines, Line{
			Content: content,
			End:     trailing,
		})
	}
	// Note: if start == len(data), data ended with a newline, no remaining content

	return lines
}

// MarshalJSON implements custom JSON serialization for Record.
func (r Record) MarshalJSON() ([]byte, error) {
	type recordAlias struct {
		Seq       uint64 `json:"seq"`
		Timestamp string `json:"timestamp"`
		Source    string `json:"source"`
		Content   any    `json:"content"`
		Encoding  string `json:"encoding"`
		End       string `json:"end,omitempty"`
		Truncated bool   `json:"truncated,omitempty"`
	}

	return json.Marshal(recordAlias(r))
}

// UnmarshalJSON implements custom JSON deserialization for Record.
func (r *Record) UnmarshalJSON(data []byte) error {
	type recordAlias struct {
		Seq       uint64          `json:"seq"`
		Timestamp string          `json:"timestamp"`
		Source    string          `json:"source"`
		Content   json.RawMessage `json:"content"`
		Encoding  string          `json:"encoding"`
		End       string          `json:"end,omitempty"`
		Truncated bool            `json:"truncated,omitempty"`
	}

	var alias recordAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}

	r.Seq = alias.Seq
	r.Timestamp = alias.Timestamp
	r.Source = alias.Source
	r.Encoding = alias.Encoding
	r.End = alias.End
	r.Truncated = alias.Truncated

	// Parse content based on encoding
	switch alias.Encoding {
	case "json":
		// Parse as native JSON value
		var parsed any
		if err := json.Unmarshal(alias.Content, &parsed); err != nil {
			return err
		}
		r.Content = parsed
	case "text", "base64":
		// Parse as string
		var str string
		if err := json.Unmarshal(alias.Content, &str); err != nil {
			return err
		}
		r.Content = str
	default:
		// Unknown encoding, store as raw string
		var str string
		if err := json.Unmarshal(alias.Content, &str); err != nil {
			// If it's not a string, store as generic value
			var parsed any
			if err := json.Unmarshal(alias.Content, &parsed); err != nil {
				return err
			}
			r.Content = parsed
		} else {
			r.Content = str
		}
	}

	return nil
}

// ToJSON serializes the record to JSON bytes.
func (r Record) ToJSON() ([]byte, error) {
	return json.Marshal(r)
}

// ContentString returns the content as a string.
// For text and base64 encoding, returns the string directly.
// For json encoding, returns the JSON representation.
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

// splitTrailingCRLF splits data into content and trailing CR/LF.
// Returns (content, trailing) where trailing contains only CR and LF characters.
func splitTrailingCRLF(data []byte) ([]byte, []byte) {
	end := len(data)
	for end > 0 {
		if data[end-1] == '\r' || data[end-1] == '\n' {
			end--
		} else {
			break
		}
	}
	return data[:end], data[end:]
}
