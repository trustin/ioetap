package recorder

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNewRecord_UTF8Content(t *testing.T) {
	timestamp := time.Date(2024, 1, 15, 10, 30, 45, 123000000, time.UTC)
	data := []byte("Hello, World!")

	record := NewRecord(0, timestamp, "stdout", data)

	if record.Seq != 0 {
		t.Errorf("expected seq 0, got %d", record.Seq)
	}
	if record.Timestamp != "2024-01-15T10:30:45.123Z" {
		t.Errorf("expected timestamp 2024-01-15T10:30:45.123Z, got %s", record.Timestamp)
	}
	if record.Source != "stdout" {
		t.Errorf("expected source stdout, got %s", record.Source)
	}
	contentStr, ok := record.Content.(string)
	if !ok {
		t.Errorf("expected content to be string, got %T", record.Content)
	} else if contentStr != "Hello, World!" {
		t.Errorf("expected content 'Hello, World!', got %s", contentStr)
	}
	if record.Encoding != "text" {
		t.Errorf("expected encoding text, got %s", record.Encoding)
	}
}

func TestNewRecord_NonUTF8Content(t *testing.T) {
	timestamp := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)
	// Invalid UTF-8 sequence
	data := []byte{0xff, 0xfe, 0x00, 0x01}

	record := NewRecord(1, timestamp, "stderr", data)

	if record.Seq != 1 {
		t.Errorf("expected seq 1, got %d", record.Seq)
	}
	if record.Source != "stderr" {
		t.Errorf("expected source stderr, got %s", record.Source)
	}
	if record.Encoding != "base64" {
		t.Errorf("expected encoding base64, got %s", record.Encoding)
	}
	// Base64 of {0xff, 0xfe, 0x00, 0x01} is "//4AAQ=="
	contentStr, ok := record.Content.(string)
	if !ok {
		t.Errorf("expected content to be string, got %T", record.Content)
	} else if contentStr != "//4AAQ==" {
		t.Errorf("expected base64 content '//4AAQ==', got %s", contentStr)
	}
}

func TestNewRecord_EmptyContent(t *testing.T) {
	timestamp := time.Now()
	data := []byte{}

	record := NewRecord(0, timestamp, "stdin", data)

	contentStr, ok := record.Content.(string)
	if !ok {
		t.Errorf("expected content to be string, got %T", record.Content)
	} else if contentStr != "" {
		t.Errorf("expected empty content, got %s", contentStr)
	}
	if record.Encoding != "text" {
		t.Errorf("expected encoding text for empty content, got %s", record.Encoding)
	}
}

func TestNewRecord_TimestampFormat(t *testing.T) {
	tests := []struct {
		input    time.Time
		expected string
	}{
		{
			time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			"2024-01-01T00:00:00.000Z",
		},
		{
			time.Date(2024, 12, 31, 23, 59, 59, 999000000, time.UTC),
			"2024-12-31T23:59:59.999Z",
		},
		{
			// Non-UTC timezone should be converted to UTC
			time.Date(2024, 6, 15, 12, 0, 0, 500000000, time.FixedZone("EST", -5*3600)),
			"2024-06-15T17:00:00.500Z",
		},
	}

	for _, tc := range tests {
		record := NewRecord(0, tc.input, "stdout", []byte("test"))
		if record.Timestamp != tc.expected {
			t.Errorf("for input %v, expected %s, got %s", tc.input, tc.expected, record.Timestamp)
		}
	}
}

func TestRecord_ToJSON(t *testing.T) {
	timestamp := time.Date(2024, 1, 15, 10, 30, 45, 123000000, time.UTC)
	record := NewRecord(42, timestamp, "stdout", []byte("test data"))

	jsonData, err := record.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	// Parse it back
	var parsed Record
	if err := json.Unmarshal(jsonData, &parsed); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if parsed.Seq != 42 {
		t.Errorf("expected seq 42, got %d", parsed.Seq)
	}
	if parsed.Timestamp != "2024-01-15T10:30:45.123Z" {
		t.Errorf("expected timestamp 2024-01-15T10:30:45.123Z, got %s", parsed.Timestamp)
	}
	if parsed.Source != "stdout" {
		t.Errorf("expected source stdout, got %s", parsed.Source)
	}
	contentStr, ok := parsed.Content.(string)
	if !ok {
		t.Errorf("expected content to be string, got %T", parsed.Content)
	} else if contentStr != "test data" {
		t.Errorf("expected content 'test data', got %s", contentStr)
	}
	if parsed.Encoding != "text" {
		t.Errorf("expected encoding text, got %s", parsed.Encoding)
	}
}

func TestRecord_JSONFieldNames(t *testing.T) {
	timestamp := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)
	record := NewRecord(0, timestamp, "stdout", []byte("test"))

	jsonData, err := record.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	// Parse as generic map to check field names
	var m map[string]interface{}
	if err := json.Unmarshal(jsonData, &m); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	expectedFields := []string{"seq", "timestamp", "source", "content", "encoding"}
	for _, field := range expectedFields {
		if _, ok := m[field]; !ok {
			t.Errorf("expected field %s not found in JSON", field)
		}
	}

	if len(m) != len(expectedFields) {
		t.Errorf("expected %d fields, got %d", len(expectedFields), len(m))
	}
}

// JSON encoding tests

func TestNewRecord_JSONObject(t *testing.T) {
	timestamp := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)
	data := []byte(`{"key":"value"}`)

	record := NewRecord(0, timestamp, "stdout", data)

	if record.Encoding != "json" {
		t.Errorf("expected encoding json, got %s", record.Encoding)
	}

	contentMap, ok := record.Content.(map[string]any)
	if !ok {
		t.Fatalf("expected content to be map[string]any, got %T", record.Content)
	}
	if contentMap["key"] != "value" {
		t.Errorf("expected content key='value', got %v", contentMap["key"])
	}
}

func TestNewRecord_JSONArray(t *testing.T) {
	timestamp := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)
	data := []byte(`[1,2,3]`)

	record := NewRecord(0, timestamp, "stdout", data)

	if record.Encoding != "json" {
		t.Errorf("expected encoding json, got %s", record.Encoding)
	}

	contentArr, ok := record.Content.([]any)
	if !ok {
		t.Fatalf("expected content to be []any, got %T", record.Content)
	}
	if len(contentArr) != 3 {
		t.Errorf("expected 3 elements, got %d", len(contentArr))
	}
	if contentArr[0] != float64(1) || contentArr[1] != float64(2) || contentArr[2] != float64(3) {
		t.Errorf("expected [1,2,3], got %v", contentArr)
	}
}

func TestNewRecord_JSONNumber(t *testing.T) {
	timestamp := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)
	data := []byte(`42`)

	record := NewRecord(0, timestamp, "stdout", data)

	if record.Encoding != "json" {
		t.Errorf("expected encoding json, got %s", record.Encoding)
	}

	contentNum, ok := record.Content.(float64)
	if !ok {
		t.Fatalf("expected content to be float64, got %T", record.Content)
	}
	if contentNum != 42 {
		t.Errorf("expected 42, got %v", contentNum)
	}
}

func TestNewRecord_JSONString(t *testing.T) {
	timestamp := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)
	data := []byte(`"text"`)

	record := NewRecord(0, timestamp, "stdout", data)

	if record.Encoding != "json" {
		t.Errorf("expected encoding json, got %s", record.Encoding)
	}

	contentStr, ok := record.Content.(string)
	if !ok {
		t.Fatalf("expected content to be string, got %T", record.Content)
	}
	if contentStr != "text" {
		t.Errorf("expected 'text', got %v", contentStr)
	}
}

func TestNewRecord_JSONBoolean(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{`true`, true},
		{`false`, false},
	}

	timestamp := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)

	for _, tc := range tests {
		record := NewRecord(0, timestamp, "stdout", []byte(tc.input))

		if record.Encoding != "json" {
			t.Errorf("for %s: expected encoding json, got %s", tc.input, record.Encoding)
		}

		contentBool, ok := record.Content.(bool)
		if !ok {
			t.Errorf("for %s: expected content to be bool, got %T", tc.input, record.Content)
			continue
		}
		if contentBool != tc.expected {
			t.Errorf("for %s: expected %v, got %v", tc.input, tc.expected, contentBool)
		}
	}
}

func TestNewRecord_JSONNull(t *testing.T) {
	timestamp := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)
	data := []byte(`null`)

	record := NewRecord(0, timestamp, "stdout", data)

	if record.Encoding != "json" {
		t.Errorf("expected encoding json, got %s", record.Encoding)
	}

	if record.Content != nil {
		t.Errorf("expected nil content, got %v", record.Content)
	}
}

func TestNewRecord_JSONWithWhitespace(t *testing.T) {
	timestamp := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)
	data := []byte("  {\"a\":1}  \n")

	record := NewRecord(0, timestamp, "stdout", data)

	if record.Encoding != "json" {
		t.Errorf("expected encoding json, got %s", record.Encoding)
	}

	contentMap, ok := record.Content.(map[string]any)
	if !ok {
		t.Fatalf("expected content to be map[string]any, got %T", record.Content)
	}
	if contentMap["a"] != float64(1) {
		t.Errorf("expected a=1, got %v", contentMap["a"])
	}
}

func TestNewRecord_JSONScientificNotation(t *testing.T) {
	timestamp := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)
	data := []byte(`1e10`)

	record := NewRecord(0, timestamp, "stdout", data)

	if record.Encoding != "json" {
		t.Errorf("expected encoding json, got %s", record.Encoding)
	}

	contentNum, ok := record.Content.(float64)
	if !ok {
		t.Fatalf("expected content to be float64, got %T", record.Content)
	}
	if contentNum != 1e10 {
		t.Errorf("expected 1e10, got %v", contentNum)
	}
}

func TestNewRecord_JSONWithUnicode(t *testing.T) {
	timestamp := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)
	data := []byte(`{"emoji":"😀"}`)

	record := NewRecord(0, timestamp, "stdout", data)

	if record.Encoding != "json" {
		t.Errorf("expected encoding json, got %s", record.Encoding)
	}

	contentMap, ok := record.Content.(map[string]any)
	if !ok {
		t.Fatalf("expected content to be map[string]any, got %T", record.Content)
	}
	if contentMap["emoji"] != "😀" {
		t.Errorf("expected emoji='😀', got %v", contentMap["emoji"])
	}
}

func TestNewRecord_InvalidJSON(t *testing.T) {
	timestamp := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)
	data := []byte(`{incomplete`)

	record := NewRecord(0, timestamp, "stdout", data)

	if record.Encoding != "text" {
		t.Errorf("expected encoding text, got %s", record.Encoding)
	}

	contentStr, ok := record.Content.(string)
	if !ok {
		t.Fatalf("expected content to be string, got %T", record.Content)
	}
	if contentStr != `{incomplete` {
		t.Errorf("expected '{incomplete', got %v", contentStr)
	}
}

func TestNewRecord_JSONWithTrailingText(t *testing.T) {
	timestamp := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)
	data := []byte(`{"a":1}blah`)

	record := NewRecord(0, timestamp, "stdout", data)

	// json.Valid rejects this, so it falls back to text
	if record.Encoding != "text" {
		t.Errorf("expected encoding text, got %s", record.Encoding)
	}
}

func TestNewRecord_MultipleJSONValues(t *testing.T) {
	timestamp := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)
	data := []byte(`{"a":1}{"b":2}`)

	record := NewRecord(0, timestamp, "stdout", data)

	// json.Valid rejects multiple JSON values, so it falls back to text
	if record.Encoding != "text" {
		t.Errorf("expected encoding text, got %s", record.Encoding)
	}
}

func TestNewRecord_PlainText(t *testing.T) {
	timestamp := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)
	data := []byte(`hello world`)

	record := NewRecord(0, timestamp, "stdout", data)

	if record.Encoding != "text" {
		t.Errorf("expected encoding text, got %s", record.Encoding)
	}

	contentStr, ok := record.Content.(string)
	if !ok {
		t.Fatalf("expected content to be string, got %T", record.Content)
	}
	if contentStr != `hello world` {
		t.Errorf("expected 'hello world', got %v", contentStr)
	}
}

func TestNewRecord_WhitespaceOnly(t *testing.T) {
	timestamp := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)
	data := []byte("   ")

	record := NewRecord(0, timestamp, "stdout", data)

	// Whitespace only is not valid JSON, falls back to text
	if record.Encoding != "text" {
		t.Errorf("expected encoding text, got %s", record.Encoding)
	}

	contentStr, ok := record.Content.(string)
	if !ok {
		t.Fatalf("expected content to be string, got %T", record.Content)
	}
	if contentStr != "   " {
		t.Errorf("expected '   ', got %q", contentStr)
	}
}

func TestRecord_JSONRoundTrip(t *testing.T) {
	timestamp := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)
	originalData := []byte(`{"key":"value","num":42}`)

	record := NewRecord(0, timestamp, "stdout", originalData)

	// Serialize to JSON
	jsonData, err := record.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	// Deserialize back
	var parsed Record
	if err := json.Unmarshal(jsonData, &parsed); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if parsed.Encoding != "json" {
		t.Errorf("expected encoding json, got %s", parsed.Encoding)
	}

	contentMap, ok := parsed.Content.(map[string]any)
	if !ok {
		t.Fatalf("expected content to be map[string]any, got %T", parsed.Content)
	}
	if contentMap["key"] != "value" {
		t.Errorf("expected key='value', got %v", contentMap["key"])
	}
	if contentMap["num"] != float64(42) {
		t.Errorf("expected num=42, got %v", contentMap["num"])
	}
}

func TestRecord_ContentString(t *testing.T) {
	timestamp := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)

	tests := []struct {
		name     string
		data     []byte
		expected string
	}{
		{
			name:     "text content",
			data:     []byte("hello"),
			expected: "hello",
		},
		{
			name:     "json object",
			data:     []byte(`{"a":1}`),
			expected: `{"a":1}`,
		},
		{
			name:     "json number",
			data:     []byte(`42`),
			expected: `42`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			record := NewRecord(0, timestamp, "stdout", tc.data)
			result := record.ContentString()
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestNewRecord_TextWithEnd(t *testing.T) {
	timestamp := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)

	tests := []struct {
		name            string
		data            []byte
		expectedContent string
		expectedEnd     string
	}{
		{
			name:            "trailing LF",
			data:            []byte("hello\n"),
			expectedContent: "hello",
			expectedEnd:     "\n",
		},
		{
			name:            "trailing CR",
			data:            []byte("hello\r"),
			expectedContent: "hello",
			expectedEnd:     "\r",
		},
		{
			name:            "trailing CRLF",
			data:            []byte("hello\r\n"),
			expectedContent: "hello",
			expectedEnd:     "\r\n",
		},
		{
			name:            "trailing CR-CR-LF",
			data:            []byte("hello\r\r\n"),
			expectedContent: "hello",
			expectedEnd:     "\r\r\n",
		},
		{
			name:            "no trailing newline",
			data:            []byte("hello"),
			expectedContent: "hello",
			expectedEnd:     "",
		},
		{
			name:            "internal CR preserved",
			data:            []byte("hello\rworld\n"),
			expectedContent: "hello\rworld",
			expectedEnd:     "\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			record := NewRecord(0, timestamp, "stdout", tc.data)

			if record.Encoding != "text" {
				t.Errorf("expected encoding text, got %s", record.Encoding)
			}

			contentStr, ok := record.Content.(string)
			if !ok {
				t.Fatalf("expected content to be string, got %T", record.Content)
			}
			if contentStr != tc.expectedContent {
				t.Errorf("content: expected %q, got %q", tc.expectedContent, contentStr)
			}
			if record.End != tc.expectedEnd {
				t.Errorf("end: expected %q, got %q", tc.expectedEnd, record.End)
			}
		})
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected []struct {
			content string
			end     string
		}
	}{
		{
			name: "single line with LF",
			data: []byte("hello\n"),
			expected: []struct {
				content string
				end     string
			}{
				{"hello", "\n"},
			},
		},
		{
			name: "single line with CRLF",
			data: []byte("hello\r\n"),
			expected: []struct {
				content string
				end     string
			}{
				{"hello", "\r\n"},
			},
		},
		{
			name: "single line without newline",
			data: []byte("hello"),
			expected: []struct {
				content string
				end     string
			}{
				{"hello", ""},
			},
		},
		{
			name: "two lines with LF",
			data: []byte("line1\nline2\n"),
			expected: []struct {
				content string
				end     string
			}{
				{"line1", "\n"},
				{"line2", "\n"},
			},
		},
		{
			name: "two lines with CRLF",
			data: []byte("line1\r\nline2\r\n"),
			expected: []struct {
				content string
				end     string
			}{
				{"line1", "\r\n"},
				{"line2", "\r\n"},
			},
		},
		{
			name: "two lines last without newline",
			data: []byte("line1\nline2"),
			expected: []struct {
				content string
				end     string
			}{
				{"line1", "\n"},
				{"line2", ""},
			},
		},
		{
			name: "three lines",
			data: []byte("a\nb\nc\n"),
			expected: []struct {
				content string
				end     string
			}{
				{"a", "\n"},
				{"b", "\n"},
				{"c", "\n"},
			},
		},
		{
			name: "internal CR preserved",
			data: []byte("hello\rworld\n"),
			expected: []struct {
				content string
				end     string
			}{
				{"hello\rworld", "\n"},
			},
		},
		{
			name: "empty lines",
			data: []byte("\n\n"),
			expected: []struct {
				content string
				end     string
			}{
				{"", "\n"},
				{"", "\n"},
			},
		},
		{
			name: "mixed line endings",
			data: []byte("a\nb\r\nc\n"),
			expected: []struct {
				content string
				end     string
			}{
				{"a", "\n"},
				{"b", "\r\n"},
				{"c", "\n"},
			},
		},
		{
			name: "empty data",
			data: []byte{},
			expected: []struct {
				content string
				end     string
			}{
				{"", ""},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lines := SplitLines(tc.data)

			if len(lines) != len(tc.expected) {
				t.Fatalf("expected %d lines, got %d", len(tc.expected), len(lines))
			}

			for i, exp := range tc.expected {
				if string(lines[i].Content) != exp.content {
					t.Errorf("line %d: content expected %q, got %q", i, exp.content, string(lines[i].Content))
				}
				if string(lines[i].End) != exp.end {
					t.Errorf("line %d: end expected %q, got %q", i, exp.end, string(lines[i].End))
				}
			}
		})
	}
}

func TestNewRecord_EndOmittedInJSON(t *testing.T) {
	timestamp := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)

	// Record without trailing newline should not have "end" in JSON
	record := NewRecord(0, timestamp, "stdout", []byte("hello"))
	jsonData, err := record.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	jsonStr := string(jsonData)
	if strings.Contains(jsonStr, `"end"`) {
		t.Errorf("JSON should not contain 'end' field when empty, got: %s", jsonStr)
	}

	// Record with trailing newline should have "end" in JSON
	record2 := NewRecord(0, timestamp, "stdout", []byte("hello\n"))
	jsonData2, err := record2.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	jsonStr2 := string(jsonData2)
	if !strings.Contains(jsonStr2, `"end":"\n"`) {
		t.Errorf("JSON should contain 'end' field, got: %s", jsonStr2)
	}
}

func TestRecord_TruncatedField(t *testing.T) {
	timestamp := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)

	// Record without truncation should not have "truncated" in JSON
	record := NewRecord(0, timestamp, "stdout", []byte("hello\n"))
	jsonData, err := record.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	jsonStr := string(jsonData)
	if strings.Contains(jsonStr, `"truncated"`) {
		t.Errorf("JSON should not contain 'truncated' field when false, got: %s", jsonStr)
	}

	// Record with truncation should have "truncated": true in JSON
	record2 := NewRecord(0, timestamp, "stdout", []byte("hello\n"))
	record2.Truncated = true
	jsonData2, err := record2.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	jsonStr2 := string(jsonData2)
	if !strings.Contains(jsonStr2, `"truncated":true`) {
		t.Errorf("JSON should contain 'truncated':true field, got: %s", jsonStr2)
	}
}

func TestRecord_TruncatedDeserialization(t *testing.T) {
	// Test deserializing a record with truncated: true
	jsonData := `{"seq":0,"timestamp":"2024-01-15T10:30:45.000Z","source":"stdout","content":"hello","encoding":"text","end":"\n","truncated":true}`

	var record Record
	if err := json.Unmarshal([]byte(jsonData), &record); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if !record.Truncated {
		t.Error("expected Truncated to be true")
	}
	if record.Content != "hello" {
		t.Errorf("expected content 'hello', got %v", record.Content)
	}
	if record.End != "\n" {
		t.Errorf("expected end '\\n', got %q", record.End)
	}

	// Test deserializing a record without truncated field (should default to false)
	jsonData2 := `{"seq":0,"timestamp":"2024-01-15T10:30:45.000Z","source":"stdout","content":"hello","encoding":"text"}`

	var record2 Record
	if err := json.Unmarshal([]byte(jsonData2), &record2); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if record2.Truncated {
		t.Error("expected Truncated to be false")
	}
}
