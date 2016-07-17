package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
)

func TestFilterChangelog(t *testing.T) {
	f, err := ioutil.TempFile("", "filter-changelog-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	fmt.Fprintln(f, `wit (2.31a-3) unstable; urgency=medium

  [ Chris Lamb ]
  * Fix for “wit: please make the build reproducible” (Closes: #831331)

  [ Michael Stapelberg ]

 -- Michael Stapelberg <stapelberg@debian.org>  Sat, 16 Jul 2016 20:39:13 +0200

wit (2.31a-2) unstable; urgency=low

  [ Tobias Gruetzmacher ]
  * Add zlib support (Closes: #815710)
  * Don’t link wfuse against libdl

 -- Michael Stapelberg <stapelberg@debian.org>  Tue, 23 Feb 2016 23:40:46 +0100`)
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	if err := filterChangelog(f.Name()); err != nil {
		t.Fatal(err)
	}

	content, err := ioutil.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}

	if got, want := string(content), `wit (2.31a-3) unstable; urgency=medium

  [ Chris Lamb ]
  * Fix for “wit: please make the build reproducible” (Closes: #831331)

 -- Michael Stapelberg <stapelberg@debian.org>  Sat, 16 Jul 2016 20:39:13 +0200

wit (2.31a-2) unstable; urgency=low

  [ Tobias Gruetzmacher ]
  * Add zlib support (Closes: #815710)
  * Don’t link wfuse against libdl

 -- Michael Stapelberg <stapelberg@debian.org>  Tue, 23 Feb 2016 23:40:46 +0100
`; got != want {
		t.Fatalf("Changelog not filtered: got %q, want %q", got, want)
	}
}
