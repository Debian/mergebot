package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

// TODO: file a bug for this issue upstream with gbp dch, then remove this code.
func filterChangelog(path string) error {
	output, err := ioutil.TempFile(filepath.Dir(path), ".mergebot-")
	if err != nil {
		return err
	}
	// Clean up in case we don’t reach the os.Rename call.
	defer os.Remove(output.Name())
	input, err := os.Open(path)
	if err != nil {
		return err
	}
	defer input.Close()
	scanner := bufio.NewScanner(input)
	var (
		lastSection   string
		copyOnly      bool
		lastLineEmpty bool
	)
	for scanner.Scan() {
		if copyOnly {
			fmt.Fprintln(output, scanner.Text())
			continue
		}
		lineEmpty := (scanner.Text() == "")
		// Defer printing section headers until the first entry of the section.
		if strings.HasPrefix(scanner.Text(), "  [ ") {
			lastSection = scanner.Text()
			continue
		}
		if strings.HasPrefix(scanner.Text(), "  * ") {
			fmt.Fprintln(output, lastSection)
		}
		// Only modify the most recent changelog entry.
		if strings.HasPrefix(scanner.Text(), " -- ") {
			copyOnly = true
		}
		if lastLineEmpty && lineEmpty {
			// Avoid printing more than one empty line at a time.
		} else {
			fmt.Fprintln(output, scanner.Text())
		}
		lastLineEmpty = lineEmpty
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	return os.Rename(output.Name(), path)
}
