package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/trustin/ioetap/internal/cli"
	"github.com/trustin/ioetap/internal/process"
	"github.com/trustin/ioetap/internal/recorder"
	"github.com/trustin/ioetap/internal/version"
)

func main() {
	os.Exit(run())
}

func run() int {
	// Handle --version / -v before parsing other arguments
	if len(os.Args) > 1 {
		arg := os.Args[1]
		if arg == "--version" || arg == "-v" {
			fmt.Println(version.Info())
			return 0
		}
	}

	opts, err := cli.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Usage: ioetap [options] -- <command> [args...]\n")
		fmt.Fprintf(os.Stderr, "       ioetap <command> [args...]\n")
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		fmt.Fprintf(os.Stderr, "  --out=<file>             Output file (default: <basename>-<pid>.jsonl)\n")
		fmt.Fprintf(os.Stderr, "  --max-line-length=<n>    Max bytes per line (0=unlimited, default: 16MiB)\n")
		fmt.Fprintf(os.Stderr, "  --version, -v            Show version information\n")
		fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
		return 1
	}

	// Start child process
	ctx := context.Background()
	proc, err := process.Start(ctx, opts.Command, opts.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ioetap: %v\n", err)
		return 1
	}

	// Determine output filename
	var filename string
	if opts.OutputFile != "" {
		filename = opts.OutputFile
	} else {
		// Default: <basename>-<pid>.jsonl
		basename := filepath.Base(opts.Command)
		filename = fmt.Sprintf("%s-%d.jsonl", basename, proc.PID())
	}

	rec, err := recorder.NewRecorder(filename, opts.MaxLineLength)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ioetap: %v\n", err)
		_ = proc.Signal(os.Kill)
		proc.Wait()
		return 1
	}
	defer rec.Close()

	// Set up signal forwarding
	sigChan := process.ForwardSignals(proc)
	defer process.StopForwardingSignals(sigChan)

	// Wait group for stdout/stderr goroutines only
	// (stdin goroutine is not included because it blocks on os.Stdin.Read()
	// which cannot be interrupted when the child process exits)
	var wg sync.WaitGroup

	// Forward stdin with recording (not in WaitGroup because os.Stdin.Read()
	// blocks and cannot be interrupted when the child process exits)
	go func() {
		defer proc.Stdin.Close()
		_ = rec.CopyAndRecord(recorder.Stdin, os.Stdin, proc.Stdin)
	}()

	// Forward stdout with recording
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = rec.CopyAndRecord(recorder.Stdout, proc.Stdout, os.Stdout)
	}()

	// Forward stderr with recording
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = rec.CopyAndRecord(recorder.Stderr, proc.Stderr, os.Stderr)
	}()

	// Wait for stdout/stderr goroutines to finish first.
	// They will finish when they read EOF from the pipes, which happens
	// when the child process exits and closes its end of the pipes.
	wg.Wait()

	// Now get the exit code from the child process
	exitCode := proc.Wait()

	// Close stdin pipe (child has exited, so this just cleans up)
	proc.Stdin.Close()

	// Sync stdout/stderr to ensure all data is flushed before exit
	os.Stdout.Sync()
	os.Stderr.Sync()

	return exitCode
}
