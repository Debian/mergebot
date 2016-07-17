package main

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
)

const (
	goldenSoapPath  = "testdata/831331.soap"
	goldenPatchPath = "testdata/831331.patch"
)

func TestGetMostRecentPatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", `multipart/related; type="text/xml"; start="<main_envelope>"; boundary="_----------=_146851316918670990"`)
		http.ServeFile(w, r, goldenSoapPath)
	}))
	defer ts.Close()

	patch, err := getMostRecentPatch(ts.URL, "831331")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if got, want := patch.Author, "Chris Lamb <lamby@debian.org>"; got != want {
		t.Fatalf("Incorrect patch author: got %q, want %q", got, want)
	}

	if got, want := patch.Subject, "wit: please make the build reproducible"; got != want {
		t.Fatalf("Incorrect patch subject: got %q, want %q", got, want)
	}

	goldenPatch, err := ioutil.ReadFile(goldenPatchPath)
	if err != nil {
		t.Fatalf("Could not read golden patch data from %q for comparison: %v", goldenPatchPath, err)
	}

	if !bytes.Equal(patch.Data, goldenPatch) {
		t.Fatalf("Patch data parsed from %q does not match %q", goldenSoapPath, goldenPatchPath)
	}
}
