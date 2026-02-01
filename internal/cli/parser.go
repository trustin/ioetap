package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// DefaultMaxLineLength is the default maximum bytes per recorded line (16 MiB).
const DefaultMaxLineLength = 16 * 1024 * 1024

// Options holds the parsed command-line options.
type Options struct {
	OutputFile    string   // --out value (empty = default naming)
	MaxLineLength int      // --max-line-length value (0 = unlimited, default: 16 MiB)
	Command       string   // First arg after --
	Args          []string // Remaining args after --
}

// Parse parses command-line arguments and returns Options.
// Supports two modes:
//   - With options: ioetap [options] -- <command> [args...]
//   - Without options (backward compatible): ioetap <command> [args...]
func Parse(args []string) (*Options, error) {
	if len(args) == 0 {
		return nil, errors.New("no command specified")
	}

	// Find -- separator position
	separatorIdx := -1
	for i, arg := range args {
		if arg == "--" {
			separatorIdx = i
			break
		}
	}

	opts := &Options{
		MaxLineLength: DefaultMaxLineLength,
	}

	if separatorIdx == -1 {
		// No separator found
		// If first arg starts with -, it's an option and requires separator
		if strings.HasPrefix(args[0], "-") {
			// Check if it's a known option to give a better error message
			if isKnownOption(args[0]) {
				return nil, errors.New("use -- separator when specifying options")
			}
			return nil, fmt.Errorf("unknown option: %s", args[0])
		}
		// Backward compatible mode: treat all args as command and args
		opts.Command = args[0]
		if len(args) > 1 {
			opts.Args = args[1:]
		}
		return opts, nil
	}

	// Parse options before --
	optionArgs := args[:separatorIdx]
	if err := parseOptions(opts, optionArgs); err != nil {
		return nil, err
	}

	// Parse command and args after --
	commandArgs := args[separatorIdx+1:]
	if len(commandArgs) == 0 {
		return nil, errors.New("no command specified")
	}

	opts.Command = commandArgs[0]
	if len(commandArgs) > 1 {
		opts.Args = commandArgs[1:]
	}

	return opts, nil
}

// parseOptions parses the options before the -- separator.
func parseOptions(opts *Options, args []string) error {
	for i := 0; i < len(args); i++ {
		arg := args[i]

		if !strings.HasPrefix(arg, "-") {
			// Non-option argument before -- means user forgot separator
			return fmt.Errorf("use -- separator when specifying options (found: %s)", arg)
		}

		// Handle --key=value format
		if strings.HasPrefix(arg, "--") && strings.Contains(arg, "=") {
			parts := strings.SplitN(arg, "=", 2)
			key := parts[0]
			value := parts[1]

			switch key {
			case "--out":
				opts.OutputFile = value
			case "--max-line-length":
				n, err := strconv.Atoi(value)
				if err != nil {
					return fmt.Errorf("--max-line-length requires an integer value: %s", value)
				}
				if n < 0 {
					return errors.New("--max-line-length cannot be negative")
				}
				opts.MaxLineLength = n
			default:
				return fmt.Errorf("unknown option: %s", key)
			}
			continue
		}

		// Handle --key value format
		switch arg {
		case "--out":
			if i+1 >= len(args) {
				return errors.New("--out requires a value")
			}
			nextArg := args[i+1]
			// Check if next arg looks like another option or is the separator
			if nextArg == "--" || (strings.HasPrefix(nextArg, "-") && !isPathLike(nextArg)) {
				return errors.New("--out requires a value")
			}
			opts.OutputFile = nextArg
			i++ // Skip the value
		case "--max-line-length":
			if i+1 >= len(args) {
				return errors.New("--max-line-length requires a value")
			}
			nextArg := args[i+1]
			if nextArg == "--" || strings.HasPrefix(nextArg, "-") {
				return errors.New("--max-line-length requires a value")
			}
			n, err := strconv.Atoi(nextArg)
			if err != nil {
				return fmt.Errorf("--max-line-length requires an integer value: %s", nextArg)
			}
			if n < 0 {
				return errors.New("--max-line-length cannot be negative")
			}
			opts.MaxLineLength = n
			i++ // Skip the value
		default:
			return fmt.Errorf("unknown option: %s", arg)
		}
	}

	return nil
}

// isPathLike checks if a string looks like a file path rather than an option.
// This allows values like "-output.jsonl" or "./--weird-file.jsonl".
func isPathLike(s string) bool {
	// If it contains a path separator or file extension, it's likely a path
	return strings.Contains(s, "/") || strings.Contains(s, ".")
}

// isKnownOption checks if the argument is a known option (with or without value).
func isKnownOption(arg string) bool {
	if arg == "--out" || arg == "--max-line-length" {
		return true
	}
	if strings.HasPrefix(arg, "--out=") || strings.HasPrefix(arg, "--max-line-length=") {
		return true
	}
	return false
}
