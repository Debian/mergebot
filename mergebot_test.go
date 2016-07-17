package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Debian/mergebot/loggedexec"
)

var (
	skipTestCleanup = flag.Bool("skip_test_cleanup", false, "Skip cleaning up the temporary directory in which the test case works for investigating what went wrong.")
)

func init() {
	flag.Parse()
	if *filterChangelogMode {
		if flag.NArg() != 1 {
			log.Fatalf("Syntax: %s -filter_changelog <path>", os.Args[0])
		}

		if err := filterChangelog(flag.Arg(0)); err != nil {
			log.Fatal(err)
		}

		os.Exit(0)
	}
}

func TestMergeAndBuild(t *testing.T) {
	os.Setenv("DEBFULLNAME", "Test Case")
	os.Setenv("DEBEMAIL", "test@case")

	flag.Set("source_package", "min")
	flag.Set("bug", "1")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", `multipart/related; type="text/xml"; start="<main_envelope>"; boundary="_----------=_146851316918670990"`)
		http.ServeFile(w, r, "testdata/minimal.soap")
	}))
	defer ts.Close()

	tempDir, err := ioutil.TempDir("", "test-merge-and-build-")
	if err != nil {
		t.Fatal(err)
	}
	if *skipTestCleanup {
		t.Logf("Not cleaning up temporary directory %q as -skip_test_cleanup was specified", tempDir)
	} else {
		defer os.RemoveAll(tempDir)
	}

	// To make mergeAndBuild() place its temporary directory inside the test’s
	os.Setenv("TMPDIR", tempDir)

	if err := exec.Command("cp", "-r", "testdata/minimal-debian-package", tempDir).Run(); err != nil {
		t.Fatal(err)
	}

	packageDir := filepath.Join(tempDir, "minimal-debian-package")

	// Initialize git repository for the packaging.
	for _, args := range [][]string{
		[]string{"init"},
		[]string{"add", "."},
		[]string{"commit", "-a", "-m", "Initial commit"},
		[]string{"tag", "debian/1.0"},
		[]string{"config", "--local", "receive.denyCurrentBranch", "updateInstead"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = packageDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %v failed: %v", args, err)
		}
	}

	// divert debcheckout with a shell script
	debcheckoutDiversion := fmt.Sprintf(
		`#!/bin/sh
echo "git\tfile://%s/.git"
`, filepath.Join(tempDir, "minimal-debian-package"))
	if err := ioutil.WriteFile(filepath.Join(tempDir, "debcheckout"), []byte(debcheckoutDiversion), 0755); err != nil {
		t.Fatal(err)
	}
	os.Setenv("PATH", tempDir+":"+os.Getenv("PATH"))

	mergeTempDir, err := mergeAndBuild(ts.URL)
	if err != nil {
		t.Fatal(err)
	}

	cmd := loggedexec.Command("git", "push")
	cmd.LogDir = tempDir
	cmd.Dir = filepath.Join(mergeTempDir, "repo")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	cmd = loggedexec.Command("git", "log", "--format=%an %ae %s", "HEAD~2..")
	cmd.LogDir = tempDir
	cmd.Dir = packageDir
	output, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(output), `Test Case test@case Update changelog for 1.1 release
Chris Lamb lamby@debian.org Fix for “wit: please make the build reproducible” (Closes: #1)
`; got != want {
		t.Fatalf("Unexpected git history after push: got %q, want %q", got, want)
	}

	cmd = loggedexec.Command("git", "tag")
	cmd.LogDir = tempDir
	cmd.Dir = packageDir
	output, err = cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(output), `debian/1.0
debian/1.1
`; got != want {
		t.Fatalf("Unexpected git history after push: got %q, want %q", got, want)
	}
}
