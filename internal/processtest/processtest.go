// Package processtest runs a command's real main function in a child
// process for smoke tests, without building a separate binary.
//
// A command test package calls Main from TestMain: in the child process it
// runs the command's main function with the child's arguments; in the test
// process it runs the tests. Start launches the current test binary as the
// child with explicit arguments, and waits for the command's "serving" log
// line, which is written after the listener is bound. Nothing waits for a
// fixed delay. Every started process is killed during test cleanup if it is
// still running, so failing assertions leak no process or listener.
package processtest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// childEnvironment marks the child process.
const childEnvironment = "ADSB_TESTBENCH_PROCESS_TEST_CHILD"

// startupBudget bounds how long a child may take to bind and log; it only
// limits a hung child and never delays a healthy one.
const startupBudget = 60 * time.Second

var servingAddress = regexp.MustCompile(`msg=serving .*address=(\S+)`)

// Config reads a shipped configuration from the repository's configs
// directory, applies change to its decoded form, and writes it to a
// temporary file whose path is returned.
func Config(t *testing.T, name string, change func(document map[string]any)) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "configs", name))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	change(document)
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return WriteConfig(t, encoded)
}

// Section returns a nested object of a decoded configuration.
func Section(document map[string]any, name string) map[string]any {
	return document[name].(map[string]any)
}

// GetJSON fetches url and decodes a 200 JSON response into target.
func GetJSON(t *testing.T, url string, target any) {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d", url, response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}

// Main runs command in a child process and the tests otherwise. Call it from
// TestMain with the package's main function.
func Main(m *testing.M, command func()) {
	if os.Getenv(childEnvironment) == "1" {
		command()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// Process is one running child command.
type Process struct {
	// Address is the bound listen address, or empty when the command exited
	// before serving.
	Address string
	cmd     *exec.Cmd
	done    chan struct{}
	mu      sync.Mutex
	log     strings.Builder
	exitErr error
}

// WriteConfig writes data to a file in a temporary directory and returns
// its path.
func WriteConfig(t *testing.T, data []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// Start launches the child with args and waits until it logs that it is
// serving or exits.
func Start(t *testing.T, args ...string) *Process {
	t.Helper()

	cmd := exec.CommandContext(context.WithoutCancel(t.Context()), os.Args[0], args...)
	cmd.Env = append(os.Environ(), childEnvironment+"=1")
	process := &Process{cmd: cmd, done: make(chan struct{})}
	serving := make(chan string, 1)
	cmd.Stdout = io.Discard
	cmd.Stderr = &lineWriter{process: process, serving: serving}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		err := cmd.Wait()
		process.mu.Lock()
		process.exitErr = err
		process.mu.Unlock()
		close(process.done)
	}()
	t.Cleanup(func() {
		select {
		case <-process.done:
		default:
			_ = cmd.Process.Kill()
			<-process.done
		}
	})

	timer := time.NewTimer(startupBudget)
	defer timer.Stop()
	select {
	case address := <-serving:
		process.Address = address
	case <-process.done:
	case <-timer.C:
		t.Fatalf("command did not start within %s:\n%s", startupBudget, process.Log())
	}
	return process
}

// lineWriter records the child's standard error and reports the serving
// address from complete log lines. exec.Cmd copies into it and Wait returns
// only after the copy finished, so no output is lost.
type lineWriter struct {
	process *Process
	serving chan<- string
	partial string
}

func (w *lineWriter) Write(data []byte) (int, error) {
	w.process.mu.Lock()
	w.process.log.Write(data)
	w.process.mu.Unlock()
	w.partial += string(data)
	for {
		line, rest, found := strings.Cut(w.partial, "\n")
		if !found {
			break
		}
		w.partial = rest
		if match := servingAddress.FindStringSubmatch(line); match != nil {
			select {
			case w.serving <- match[1]:
			default:
			}
		}
	}
	return len(data), nil
}

// Log returns everything the child wrote to standard error so far.
func (p *Process) Log() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.log.String()
}

// Wait waits for the child to exit and returns its exit code.
func (p *Process) Wait(t *testing.T) int {
	t.Helper()

	timer := time.NewTimer(startupBudget)
	defer timer.Stop()
	select {
	case <-p.done:
	case <-timer.C:
		t.Fatalf("command did not exit within %s:\n%s", startupBudget, p.Log())
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if exit, ok := errors.AsType[*exec.ExitError](p.exitErr); ok {
		return exit.ExitCode()
	}
	if p.exitErr != nil {
		t.Fatalf("wait for command: %v", p.exitErr)
	}
	return 0
}

// Interrupt sends os.Interrupt to the child. It fails on platforms that
// cannot deliver it, such as Windows.
func (p *Process) Interrupt() error {
	if err := p.cmd.Process.Signal(os.Interrupt); err != nil {
		return fmt.Errorf("interrupt command: %w", err)
	}
	return nil
}
