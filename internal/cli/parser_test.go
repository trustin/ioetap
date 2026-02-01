package cli

import (
	"testing"
)

func TestParse_CommandOnly(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    *Options
		wantErr bool
	}{
		{
			name: "simple command with separator",
			args: []string{"--", "ls"},
			want: &Options{Command: "ls"},
		},
		{
			name: "command with args and separator",
			args: []string{"--", "ls", "-la"},
			want: &Options{Command: "ls", Args: []string{"-la"}},
		},
		{
			name: "backward compatible - simple command",
			args: []string{"ls"},
			want: &Options{Command: "ls"},
		},
		{
			name: "backward compatible - command with args",
			args: []string{"ls", "-l"},
			want: &Options{Command: "ls", Args: []string{"-l"}},
		},
		{
			name: "backward compatible - echo with multiple args",
			args: []string{"echo", "hello", "world"},
			want: &Options{Command: "echo", Args: []string{"hello", "world"}},
		},
		{
			name: "command starting with dash after separator",
			args: []string{"--", "-c", "script.sh"},
			want: &Options{Command: "-c", Args: []string{"script.sh"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if got.Command != tt.want.Command {
				t.Errorf("Command = %v, want %v", got.Command, tt.want.Command)
			}
			if len(got.Args) != len(tt.want.Args) {
				t.Errorf("Args length = %v, want %v", len(got.Args), len(tt.want.Args))
				return
			}
			for i, arg := range got.Args {
				if arg != tt.want.Args[i] {
					t.Errorf("Args[%d] = %v, want %v", i, arg, tt.want.Args[i])
				}
			}
			if got.OutputFile != tt.want.OutputFile {
				t.Errorf("OutputFile = %v, want %v", got.OutputFile, tt.want.OutputFile)
			}
		})
	}
}

func TestParse_WithOutOption(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    *Options
		wantErr bool
	}{
		{
			name: "out option with equals",
			args: []string{"--out=file.jsonl", "--", "ls"},
			want: &Options{OutputFile: "file.jsonl", Command: "ls"},
		},
		{
			name: "out option with space",
			args: []string{"--out", "file.jsonl", "--", "ls", "-la"},
			want: &Options{OutputFile: "file.jsonl", Command: "ls", Args: []string{"-la"}},
		},
		{
			name: "out option with path containing spaces",
			args: []string{"--out=my file.jsonl", "--", "echo", "hello"},
			want: &Options{OutputFile: "my file.jsonl", Command: "echo", Args: []string{"hello"}},
		},
		{
			name: "out option value that looks like separator",
			args: []string{"--out=--", "--", "ls"},
			want: &Options{OutputFile: "--", Command: "ls"},
		},
		{
			name: "out option with absolute path",
			args: []string{"--out=/tmp/output.jsonl", "--", "echo", "test"},
			want: &Options{OutputFile: "/tmp/output.jsonl", Command: "echo", Args: []string{"test"}},
		},
		{
			name: "out option with relative path",
			args: []string{"--out=./output.jsonl", "--", "echo", "test"},
			want: &Options{OutputFile: "./output.jsonl", Command: "echo", Args: []string{"test"}},
		},
		{
			name: "out option with path-like value starting with dash",
			args: []string{"--out", "-output.jsonl", "--", "ls"},
			want: &Options{OutputFile: "-output.jsonl", Command: "ls"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if got.Command != tt.want.Command {
				t.Errorf("Command = %v, want %v", got.Command, tt.want.Command)
			}
			if got.OutputFile != tt.want.OutputFile {
				t.Errorf("OutputFile = %v, want %v", got.OutputFile, tt.want.OutputFile)
			}
			if len(got.Args) != len(tt.want.Args) {
				t.Errorf("Args length = %v, want %v", len(got.Args), len(tt.want.Args))
				return
			}
			for i, arg := range got.Args {
				if arg != tt.want.Args[i] {
					t.Errorf("Args[%d] = %v, want %v", i, arg, tt.want.Args[i])
				}
			}
		})
	}
}

func TestParse_Errors(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantErrMsg  string
	}{
		{
			name:       "empty args",
			args:       []string{},
			wantErrMsg: "no command specified",
		},
		{
			name:       "only separator",
			args:       []string{"--"},
			wantErrMsg: "no command specified",
		},
		{
			name:       "option without separator then command",
			args:       []string{"--out=file.jsonl", "ls"},
			wantErrMsg: "use -- separator when specifying options",
		},
		{
			name:       "option with space without separator then command",
			args:       []string{"--out", "file.jsonl", "ls"},
			wantErrMsg: "use -- separator when specifying options",
		},
		{
			name:       "unknown option with separator",
			args:       []string{"--unknown", "--", "ls"},
			wantErrMsg: "unknown option: --unknown",
		},
		{
			name:       "unknown option equals form",
			args:       []string{"--unknown=value", "--", "ls"},
			wantErrMsg: "unknown option: --unknown",
		},
		{
			name:       "unknown short option without separator",
			args:       []string{"-x", "ls"},
			wantErrMsg: "unknown option: -x",
		},
		{
			name:       "out option without value at end",
			args:       []string{"--out"},
			wantErrMsg: "use -- separator when specifying options",
		},
		{
			name:       "out option followed by separator",
			args:       []string{"--out", "--", "ls"},
			wantErrMsg: "--out requires a value",
		},
		{
			name:       "out option followed by another option",
			args:       []string{"--out", "--other", "--", "ls"},
			wantErrMsg: "--out requires a value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.args)
			if err == nil {
				t.Errorf("Parse() expected error containing %q, got nil", tt.wantErrMsg)
				return
			}
			if err.Error() != tt.wantErrMsg && !containsString(err.Error(), tt.wantErrMsg) {
				t.Errorf("Parse() error = %q, want error containing %q", err.Error(), tt.wantErrMsg)
			}
		})
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestParse_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    *Options
		wantErr bool
	}{
		{
			name: "multiple -- in command args",
			args: []string{"--", "sh", "-c", "echo --help"},
			want: &Options{Command: "sh", Args: []string{"-c", "echo --help"}},
		},
		{
			name: "command is --help (after separator)",
			args: []string{"--", "--help"},
			want: &Options{Command: "--help"},
		},
		{
			name: "empty output file with equals",
			args: []string{"--out=", "--", "ls"},
			want: &Options{OutputFile: "", Command: "ls"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if got.Command != tt.want.Command {
				t.Errorf("Command = %v, want %v", got.Command, tt.want.Command)
			}
			if got.OutputFile != tt.want.OutputFile {
				t.Errorf("OutputFile = %v, want %v", got.OutputFile, tt.want.OutputFile)
			}
			if len(got.Args) != len(tt.want.Args) {
				t.Errorf("Args length = %v, want %v", len(got.Args), len(tt.want.Args))
				return
			}
			for i, arg := range got.Args {
				if arg != tt.want.Args[i] {
					t.Errorf("Args[%d] = %v, want %v", i, arg, tt.want.Args[i])
				}
			}
		})
	}
}

func TestParse_MaxLineLengthOption(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    *Options
		wantErr bool
	}{
		{
			name: "max-line-length with equals",
			args: []string{"--max-line-length=1000", "--", "ls"},
			want: &Options{MaxLineLength: 1000, Command: "ls"},
		},
		{
			name: "max-line-length with space",
			args: []string{"--max-line-length", "2000", "--", "ls"},
			want: &Options{MaxLineLength: 2000, Command: "ls"},
		},
		{
			name: "max-line-length zero (unlimited)",
			args: []string{"--max-line-length=0", "--", "ls"},
			want: &Options{MaxLineLength: 0, Command: "ls"},
		},
		{
			name: "max-line-length combined with out",
			args: []string{"--out=test.jsonl", "--max-line-length=500", "--", "echo", "hello"},
			want: &Options{OutputFile: "test.jsonl", MaxLineLength: 500, Command: "echo", Args: []string{"hello"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if got.Command != tt.want.Command {
				t.Errorf("Command = %v, want %v", got.Command, tt.want.Command)
			}
			if got.MaxLineLength != tt.want.MaxLineLength {
				t.Errorf("MaxLineLength = %v, want %v", got.MaxLineLength, tt.want.MaxLineLength)
			}
			if got.OutputFile != tt.want.OutputFile {
				t.Errorf("OutputFile = %v, want %v", got.OutputFile, tt.want.OutputFile)
			}
			if len(got.Args) != len(tt.want.Args) {
				t.Errorf("Args length = %v, want %v", len(got.Args), len(tt.want.Args))
				return
			}
			for i, arg := range got.Args {
				if arg != tt.want.Args[i] {
					t.Errorf("Args[%d] = %v, want %v", i, arg, tt.want.Args[i])
				}
			}
		})
	}
}

func TestParse_MaxLineLengthErrors(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantErrMsg string
	}{
		{
			name:       "max-line-length negative",
			args:       []string{"--max-line-length=-1", "--", "ls"},
			wantErrMsg: "--max-line-length cannot be negative",
		},
		{
			name:       "max-line-length non-integer",
			args:       []string{"--max-line-length=abc", "--", "ls"},
			wantErrMsg: "--max-line-length requires an integer value",
		},
		{
			name:       "max-line-length float",
			args:       []string{"--max-line-length=1.5", "--", "ls"},
			wantErrMsg: "--max-line-length requires an integer value",
		},
		{
			name:       "max-line-length missing value",
			args:       []string{"--max-line-length", "--", "ls"},
			wantErrMsg: "--max-line-length requires a value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.args)
			if err == nil {
				t.Errorf("Parse() expected error containing %q, got nil", tt.wantErrMsg)
				return
			}
			if !containsString(err.Error(), tt.wantErrMsg) {
				t.Errorf("Parse() error = %q, want error containing %q", err.Error(), tt.wantErrMsg)
			}
		})
	}
}

func TestParse_DefaultMaxLineLength(t *testing.T) {
	// Test that default max line length is 16 MiB
	got, err := Parse([]string{"ls"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got.MaxLineLength != DefaultMaxLineLength {
		t.Errorf("MaxLineLength = %v, want default %v", got.MaxLineLength, DefaultMaxLineLength)
	}
	if DefaultMaxLineLength != 16*1024*1024 {
		t.Errorf("DefaultMaxLineLength = %v, want 16 MiB (%v)", DefaultMaxLineLength, 16*1024*1024)
	}
}
