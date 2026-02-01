package process

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// Process wraps an exec.Cmd with stdin/stdout/stderr pipes.
type Process struct {
	cmd    *exec.Cmd
	Stdin  io.WriteCloser
	Stdout io.ReadCloser
	Stderr io.ReadCloser
}

// Start creates and starts a new child process with the given command and arguments.
func Start(ctx context.Context, name string, args []string) (*Process, error) {
	cmd := exec.CommandContext(ctx, name, args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdin.Close()
		stdout.Close()
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		stderr.Close()
		return nil, fmt.Errorf("failed to start process: %w", err)
	}

	return &Process{
		cmd:    cmd,
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	}, nil
}

// PID returns the process ID of the child process.
func (p *Process) PID() int {
	return p.cmd.Process.Pid
}

// Signal sends a signal to the child process.
func (p *Process) Signal(sig os.Signal) error {
	return p.cmd.Process.Signal(sig)
}

// Wait waits for the process to exit and returns the exit code.
func (p *Process) Wait() int {
	err := p.cmd.Wait()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		// If we can't determine the exit code, return -1
		return -1
	}
	return 0
}

// ForwardSignals sets up signal forwarding to the child process.
// It returns a channel that will receive signals, allowing the caller to stop forwarding.
func ForwardSignals(proc *Process) chan os.Signal {
	sigChan := make(chan os.Signal, 1)

	// Forward common signals
	signal.Notify(sigChan,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGHUP,
		syscall.SIGQUIT,
		syscall.SIGUSR1,
		syscall.SIGUSR2,
	)

	go func() {
		for sig := range sigChan {
			_ = proc.Signal(sig)
		}
	}()

	return sigChan
}

// StopForwardingSignals stops signal forwarding and closes the channel.
func StopForwardingSignals(sigChan chan os.Signal) {
	signal.Stop(sigChan)
	close(sigChan)
}
