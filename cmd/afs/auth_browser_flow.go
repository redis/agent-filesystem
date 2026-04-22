package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const browserLoginTimeout = 5 * time.Minute

type browserLoginResult struct {
	Token string
	Err   error
}

func launchBrowserLoginFlow(controlPlaneURL, workspace string) (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("start browser login callback: %w", err)
	}
	defer listener.Close()

	state, err := newBrowserLoginState()
	if err != nil {
		return "", err
	}

	returnTo := "http://" + listener.Addr().String() + "/callback"
	loginURL, err := buildBrowserLoginURL(controlPlaneURL, returnTo, state, workspace)
	if err != nil {
		return "", err
	}

	resultCh := make(chan browserLoginResult, 1)
	server := &http.Server{
		Handler:           browserLoginCallbackHandler(state, resultCh),
		ReadHeaderTimeout: 5 * time.Second,
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			select {
			case resultCh <- browserLoginResult{Err: serveErr}:
			default:
			}
		}
	}()

	printBrowserLoginPrompt(loginURL)

	if err := openBrowser(loginURL); err != nil {
		fmt.Printf("\n  %s %s\n\n", clr(ansiDim, "▸"), clr(ansiDim, "Could not open the browser automatically. Open the login URL above manually."))
	}

	timeout := time.NewTimer(browserLoginTimeout)
	defer timeout.Stop()

	select {
	case result := <-resultCh:
		return strings.TrimSpace(result.Token), result.Err
	case <-timeout.C:
		return "", fmt.Errorf("browser login timed out after %s", browserLoginTimeout.Round(time.Second))
	}
}

func printBrowserLoginPrompt(loginURL string) {
	fmt.Println()
	fmt.Println("  " + clr(ansiBold, "Open this login URL in your browser:"))
	fmt.Println("  " + loginURL)
	fmt.Println("  " + clr(ansiDim, "If the browser does not open, paste the login URL into your browser."))
	fmt.Println()
}

func browserLoginCallbackHandler(expectedState string, resultCh chan<- browserLoginResult) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if strings.TrimSpace(query.Get("state")) != expectedState {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(browserLoginCompletionPage("AFS login failed", "The login response did not match this CLI session. Close this tab and try again.")))
			select {
			case resultCh <- browserLoginResult{Err: fmt.Errorf("browser login returned an unexpected state")}:
			default:
			}
			return
		}

		if message := strings.TrimSpace(query.Get("error")); message != "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(browserLoginCompletionPage("AFS login failed", message)))
			select {
			case resultCh <- browserLoginResult{Err: errors.New(message)}:
			default:
			}
			return
		}

		token := strings.TrimSpace(query.Get("token"))
		if token == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(browserLoginCompletionPage("AFS login failed", "No onboarding token was returned to the CLI.")))
			select {
			case resultCh <- browserLoginResult{Err: fmt.Errorf("browser login returned no onboarding token")}:
			default:
			}
			return
		}

		_, _ = w.Write([]byte(browserLoginCompletionPage("AFS login complete", "You can return to the terminal. The CLI will finish connecting automatically.")))
		select {
		case resultCh <- browserLoginResult{Token: token}:
		default:
		}
	})
	return mux
}

func browserLoginCompletionPage(title, message string) string {
	return `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>` + html.EscapeString(title) + `</title>
    <style>
      body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background: #f6f4ee; color: #16140f; margin: 0; padding: 32px; }
      .card { max-width: 560px; margin: 48px auto; background: #fffdf8; border: 1px solid #e9dfcc; border-radius: 18px; padding: 28px 24px; box-shadow: 0 18px 50px rgba(91, 71, 32, 0.08); }
      h1 { margin: 0 0 12px; font-size: 30px; line-height: 1.1; }
      p { margin: 0; font-size: 16px; line-height: 1.6; color: #5a503b; }
    </style>
  </head>
  <body>
    <div class="card">
      <h1>` + html.EscapeString(title) + `</h1>
      <p>` + html.EscapeString(message) + `</p>
    </div>
  </body>
</html>`
}

func buildBrowserLoginURL(controlPlaneURL, returnTo, state, workspace string) (string, error) {
	base, err := url.Parse(controlPlaneURL)
	if err != nil {
		return "", err
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/connect-cli"
	query := base.Query()
	query.Set("return_to", returnTo)
	query.Set("state", state)
	if strings.TrimSpace(workspace) != "" {
		query.Set("workspace", strings.TrimSpace(workspace))
	}
	base.RawQuery = query.Encode()
	return base.String(), nil
}

func newBrowserLoginState() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate browser login state: %w", err)
	}
	return "afs_auth_" + hex.EncodeToString(raw[:]), nil
}

func openBrowser(target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	return cmd.Start()
}
