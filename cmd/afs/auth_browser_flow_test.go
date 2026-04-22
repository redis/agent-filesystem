package main

import (
	"strings"
	"testing"
)

func TestPrintBrowserLoginPromptShowsFullURLAndHint(t *testing.T) {
	t.Helper()

	const loginURL = "https://afs.example.com/connect-cli?return_to=http%3A%2F%2F127.0.0.1%3A4444%2Fcallback&state=afs_auth_demo&workspace=ws_demo"

	out, err := captureStdout(t, func() error {
		printBrowserLoginPrompt(loginURL)
		return nil
	})
	if err != nil {
		t.Fatalf("printBrowserLoginPrompt() returned error: %v", err)
	}

	for _, want := range []string{
		"Open this login URL in your browser:",
		loginURL,
		"If the browser does not open, paste the login URL into your browser.",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("prompt output = %q, want substring %q", out, want)
		}
	}
}
