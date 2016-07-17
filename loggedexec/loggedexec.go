// loggedexec is a wrapper around os/exec which logs command
// execution, the command’s stdout/stderr into files and provides
// better error messages when command execution fails. This makes
// debugging easier for end users.
package loggedexec

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	cmdCount   int
	cmdCountMu sync.Mutex
)

// LoggedCmd is like (os/exec).Cmd, but its Run() method additionally:
//
//   * Logs each invocation’s command for human consumption.
//   * Logs each invocation’s working directory, Args, Env and timing
//     into a file.
//   * Logs each invocation’s stdout/stderr into a file.
//   * Wraps the returned error (if any) with the command and pointers
//     to the log files with more details (including the first line
//     of stdout/stderr).
//
// All files are created in LogDir.
type LoggedCmd struct {
	*exec.Cmd

	// Logger will be used to log invocation commands for human
	// consumption. Defaults to logging to os.Stderr. Use
	// ioutil.Discard to hide logs.
	Logger *log.Logger

	// LogDir is the directory in which log files will be
	// created. Defaults to os.TempDir().
	LogDir string

	// LogFmt is the prefix used for naming log files in
	// LogDir. Defaults to "%03d-" and must contain precisely one "%d"
	// which will be replaced with the invocation count.
	LogFmt string
}

// Command is like (os/exec).Command, but returns a LoggedCmd.
func Command(name string, arg ...string) *LoggedCmd {
	return &LoggedCmd{
		Cmd:    exec.Command(name, arg...),
		Logger: log.New(os.Stderr, "", log.Lshortfile),
		LogFmt: "%03d-",
	}
}

// capturingWriter captures data until a newline (\n) is seen, so that
// we can display the first log line in error messages.
type capturingWriter struct {
	Data        []byte
	newlineSeen bool
}

func (c *capturingWriter) Write(p []byte) (n int, err error) {
	if !c.newlineSeen {
		c.Data = append(c.Data, p...)
		// Start searching from the end, as newlines are more likely
		// to occur at the end.
		c.newlineSeen = (bytes.LastIndexByte(p, '\n') != -1)
	}
	return len(p), nil
}

func (c *capturingWriter) FirstLine() string {
	s := string(c.Data)
	idx := strings.IndexByte(s, '\n')
	if idx == -1 {
		return s
	}
	return s[:idx]
}

func quoteStrings(input []string) []string {
	output := make([]string, len(input))
	for idx, val := range input {
		output[idx] = `"` + val + `"`
	}
	return output
}

// Run is a wrapper around (os/exec).Cmd’s Run().
func (l *LoggedCmd) Run() error {
	commandline := strings.Join(l.Args, " ")
	l.Logger.Printf("%s", commandline)

	if l.LogDir == "" {
		l.LogDir = os.TempDir()
	}
	cmdCountMu.Lock()
	// To prevent leaking private data, only l.Args[0] goes into the
	// file name, which is readable by other users on the same system.
	logPrefix := filepath.Join(l.LogDir, fmt.Sprintf(l.LogFmt, cmdCount)+l.Args[0])
	cmdCount++
	cmdCountMu.Unlock()
	invocationLogPath := logPrefix + ".invocation.log"
	logPath := logPrefix + ".stdoutstderr.log"

	workDir := l.Dir
	if workDir == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	started := time.Now()
	invocationLog := fmt.Sprintf(
		"Execution started: %v\n"+
			"Working directory: %q\n"+
			"Command (%d elements):\n\t%s\n"+
			"Environment (%d elements):\n\t%s\n",
		started,
		workDir,
		len(l.Args),
		strings.Join(quoteStrings(l.Args), "\n\t"),
		len(l.Env),
		strings.Join(quoteStrings(l.Env), "\n\t"))
	if err := ioutil.WriteFile(invocationLogPath, []byte(invocationLog+"(Still running…)"), 0600); err != nil {
		return err
	}
	logFile, err := os.OpenFile(logPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	var cw capturingWriter
	logWriter := io.MultiWriter(logFile, &cw)
	if l.Stdout == nil {
		l.Stdout = logWriter
	} else {
		l.Stdout = io.MultiWriter(l.Stdout, logWriter)
	}
	if l.Stderr == nil {
		l.Stderr = logWriter
	} else {
		l.Stderr = io.MultiWriter(l.Stderr, logWriter)
	}
	runErr := l.Cmd.Run()
	finished := time.Now()
	invocationLog = invocationLog + fmt.Sprintf(
		"Execution finished: %v (duration: %v)",
		finished,
		finished.Sub(started))
	// Update the invocation log atomically to not lose data when
	// (e.g.) running out of disk space.
	f, err := ioutil.TempFile(filepath.Dir(invocationLogPath), ".invocation-log-")
	if err != nil {
		return err
	}
	fmt.Fprintln(f, invocationLog)
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), invocationLogPath); err != nil {
		return err
	}
	if runErr == nil {
		return nil
	}
	firstLogLine := cw.FirstLine()
	return fmt.Errorf("Running %q: %v\n"+
		"See %q for invocation details.\n"+
		"See %q for full stdout/stderr.\n"+
		"First stdout/stderr line: %q\n",
		commandline,
		runErr,
		invocationLogPath,
		logPath,
		firstLogLine)
}
