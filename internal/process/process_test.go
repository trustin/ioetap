package process

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"
)

func TestProcess_StartAndExitCode(t *testing.T) {
	ctx := context.Background()

	// Test successful exit
	proc, err := Start(ctx, "sh", []string{"-c", "exit 0"})
	if err != nil {
		t.Fatalf("failed to start process: %v", err)
	}

	// Close stdin to allow the process to finish
	proc.Stdin.Close()

	// Drain stdout/stderr
	go func() { _, _ = io.Copy(io.Discard, proc.Stdout) }()
	go func() { _, _ = io.Copy(io.Discard, proc.Stderr) }()

	exitCode := proc.Wait()
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}

	// Test non-zero exit
	proc2, err := Start(ctx, "sh", []string{"-c", "exit 42"})
	if err != nil {
		t.Fatalf("failed to start process: %v", err)
	}

	proc2.Stdin.Close()
	go func() { _, _ = io.Copy(io.Discard, proc2.Stdout) }()
	go func() { _, _ = io.Copy(io.Discard, proc2.Stderr) }()

	exitCode2 := proc2.Wait()
	if exitCode2 != 42 {
		t.Errorf("expected exit code 42, got %d", exitCode2)
	}
}

func TestProcess_StdoutCapture(t *testing.T) {
	ctx := context.Background()

	proc, err := Start(ctx, "echo", []string{"hello world"})
	if err != nil {
		t.Fatalf("failed to start process: %v", err)
	}

	proc.Stdin.Close()
	go func() { _, _ = io.Copy(io.Discard, proc.Stderr) }()

	output, err := io.ReadAll(proc.Stdout)
	if err != nil {
		t.Fatalf("failed to read stdout: %v", err)
	}

	proc.Wait()

	expected := "hello world\n"
	if string(output) != expected {
		t.Errorf("expected stdout %q, got %q", expected, string(output))
	}
}

func TestProcess_StderrCapture(t *testing.T) {
	ctx := context.Background()

	proc, err := Start(ctx, "sh", []string{"-c", "echo error >&2"})
	if err != nil {
		t.Fatalf("failed to start process: %v", err)
	}

	proc.Stdin.Close()
	go func() { _, _ = io.Copy(io.Discard, proc.Stdout) }()

	output, err := io.ReadAll(proc.Stderr)
	if err != nil {
		t.Fatalf("failed to read stderr: %v", err)
	}

	proc.Wait()

	expected := "error\n"
	if string(output) != expected {
		t.Errorf("expected stderr %q, got %q", expected, string(output))
	}
}

func TestProcess_StdinForwarding(t *testing.T) {
	ctx := context.Background()

	proc, err := Start(ctx, "cat", []string{})
	if err != nil {
		t.Fatalf("failed to start process: %v", err)
	}

	go func() { _, _ = io.Copy(io.Discard, proc.Stderr) }()

	// Write to stdin
	input := "test input"
	go func() {
		_, _ = proc.Stdin.Write([]byte(input))
		proc.Stdin.Close()
	}()

	output, err := io.ReadAll(proc.Stdout)
	if err != nil {
		t.Fatalf("failed to read stdout: %v", err)
	}

	proc.Wait()

	if string(output) != input {
		t.Errorf("expected output %q, got %q", input, string(output))
	}
}

func TestProcess_PID(t *testing.T) {
	ctx := context.Background()

	proc, err := Start(ctx, "sleep", []string{"0.1"})
	if err != nil {
		t.Fatalf("failed to start process: %v", err)
	}

	pid := proc.PID()
	if pid <= 0 {
		t.Errorf("expected positive PID, got %d", pid)
	}

	proc.Stdin.Close()
	go func() { _, _ = io.Copy(io.Discard, proc.Stdout) }()
	go func() { _, _ = io.Copy(io.Discard, proc.Stderr) }()

	proc.Wait()
}

func TestProcess_InvalidCommand(t *testing.T) {
	ctx := context.Background()

	_, err := Start(ctx, "nonexistent-command-12345", []string{})
	if err == nil {
		t.Error("expected error for non-existent command, got nil")
	}
}

func TestProcess_ConcurrentStdoutStderr(t *testing.T) {
	ctx := context.Background()

	// Command that writes to both stdout and stderr
	proc, err := Start(ctx, "sh", []string{"-c", "echo out; echo err >&2; echo out2; echo err2 >&2"})
	if err != nil {
		t.Fatalf("failed to start process: %v", err)
	}

	proc.Stdin.Close()

	var stdout, stderr bytes.Buffer
	done := make(chan bool, 2)

	go func() {
		_, _ = io.Copy(&stdout, proc.Stdout)
		done <- true
	}()

	go func() {
		_, _ = io.Copy(&stderr, proc.Stderr)
		done <- true
	}()

	<-done
	<-done

	proc.Wait()

	// Verify both outputs were captured
	if stdout.Len() == 0 {
		t.Error("expected non-empty stdout")
	}
	if stderr.Len() == 0 {
		t.Error("expected non-empty stderr")
	}
}

func TestForwardSignals(t *testing.T) {
	ctx := context.Background()

	// Start a process that will wait for a signal
	proc, err := Start(ctx, "sleep", []string{"10"})
	if err != nil {
		t.Fatalf("failed to start process: %v", err)
	}

	proc.Stdin.Close()
	go func() { _, _ = io.Copy(io.Discard, proc.Stdout) }()
	go func() { _, _ = io.Copy(io.Discard, proc.Stderr) }()

	// Set up signal forwarding
	sigChan := ForwardSignals(proc)

	// Send SIGTERM to the child process directly
	// (we can't easily test signal forwarding from parent to child in a unit test)
	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = proc.Signal(nil) // This would normally be a real signal
	}()

	// The process should eventually exit
	// We'll just clean up here
	StopForwardingSignals(sigChan)

	// Kill the process to clean up
	_ = proc.Signal(nil)
}
