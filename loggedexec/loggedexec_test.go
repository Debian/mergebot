package loggedexec

import (
	"bytes"
	"io/ioutil"
	"log"
	"regexp"
	"testing"
)

// TestErrorMessage verifies the error message contains additional
// details.
//
// TestErrorMessage must run as the first test, since it makes
// assumptions about the output file name, which depends on the number
// of commands ran so far.
func TestErrorMessage(t *testing.T) {
	cmd := Command("ls", "/tmp/nope")
	cmd.Logger = log.New(ioutil.Discard, "", 0)
	cmd.Env = []string{"LANG=C"}
	err := cmd.Run()
	if err == nil {
		t.Fatalf("Unexpectedly, running %v did not result in an error", cmd.Args)
	}
	want := `Running "ls /tmp/nope": exit status 2
See "/tmp/000-ls.invocation.log" for invocation details.
See "/tmp/000-ls.stdoutstderr.log" for full stdout/stderr.
First stdout/stderr line: "ls: cannot access /tmp/nope: No such file or directory"
`
	if got := err.Error(); got != want {
		t.Fatalf("Unexpected error message: got %q, want %q", got, want)
	}
}

// TestLogExecution verifies that command executions are logged to the
// specified Logger.
func TestLogExecution(t *testing.T) {
	cmd := Command("ls", "/tmp/nope")
	var buf bytes.Buffer
	cmd.Logger = log.New(&buf, "", 0)
	cmd.Env = []string{"LANG=C"}
	cmd.Run()
	if got, want := buf.String(), "ls /tmp/nope\n"; got != want {
		t.Fatalf("Unexpected log output: got %q, want %q", got, want)
	}
}

func TestInvocationLogFile(t *testing.T) {
	cmd := Command("ls", "/tmp/nope")
	cmd.Logger = log.New(ioutil.Discard, "", 0)
	cmd.Env = []string{"LANG=C"}
	err := cmd.Run()
	if err == nil {
		t.Fatalf("Unexpectedly, running %v did not result in an error", cmd.Args)
	}
	invocationLogRe := regexp.MustCompile(`See "([^"]+)" for invocation details`)
	matches := invocationLogRe.FindStringSubmatch(err.Error())
	if got, want := len(matches), 2; got != want {
		t.Fatalf("Unexpected number of regexp (%q) matches: got %d, want %d", invocationLogRe, got, want)
	}
	contents, err := ioutil.ReadFile(matches[1])
	if err != nil {
		t.Fatalf("Could not read invocation log: %v", err)
	}

	invocationLogContentsRe := regexp.MustCompile(
		`(?m)Execution started: .*$
Working directory: "[^"]+"$
Command \(2 elements\):$
\t"ls"$
\t"/tmp/nope"$
Environment \(1 elements\):$
\t"LANG=C"$
Execution finished: .* \(duration: [^)]+\)$`)
	if !invocationLogContentsRe.Match(contents) {
		t.Fatalf("Invocation log contents (%q) don’t match regexp %q", string(contents), invocationLogContentsRe)
	}
}

// testLogFile contains TestLogFile’s logic, so that it can be reused
// in TestTee with a slightly modified cmd.
func testLogFile(t *testing.T, cmd *LoggedCmd) {
	err := cmd.Run()
	if err == nil {
		t.Fatalf("Unexpectedly, running %v did not result in an error", cmd.Args)
	}
	stdoutLogRe := regexp.MustCompile(`See "([^"]+)" for full stdout/stderr`)
	matches := stdoutLogRe.FindStringSubmatch(err.Error())
	if got, want := len(matches), 2; got != want {
		t.Fatalf("Unexpected number of regexp (%q) matches: got %d, want %d", stdoutLogRe, got, want)
	}
	contents, err := ioutil.ReadFile(matches[1])
	if err != nil {
		t.Fatalf("Could not read stdout/stderr log: %v", err)
	}
	if got, want := string(contents), "ls: cannot access /tmp/nope: No such file or directory\n"; got != want {
		t.Fatalf("Unexpected stdout/stderr log contents: got %q, want %q", got, want)
	}
}

// TestLogFile verifies the log file referenced in the error message
// actually contains the stdout/stderr.
func TestLogFile(t *testing.T) {
	cmd := Command("ls", "/tmp/nope")
	cmd.Logger = log.New(ioutil.Discard, "", 0)
	cmd.Env = []string{"LANG=C"}
	testLogFile(t, cmd)
}

func TestTee(t *testing.T) {
	cmd := Command("ls", "/tmp/nope")
	cmd.Logger = log.New(ioutil.Discard, "", 0)
	cmd.Env = []string{"LANG=C"}
	var stdouterr bytes.Buffer
	cmd.Stdout = &stdouterr
	cmd.Stderr = &stdouterr
	testLogFile(t, cmd)
	if got, want := stdouterr.String(), "ls: cannot access /tmp/nope: No such file or directory\n"; got != want {
		t.Fatalf("Unexpected stdout/stderr buffer contents: got %q, want %q", got, want)
	}
}

func TestResetCounter(t *testing.T) {
	cmdCountMu.Lock()
	cmdCount = 0
	cmdCountMu.Unlock()
}
