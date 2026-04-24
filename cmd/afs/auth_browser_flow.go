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
			_, _ = w.Write([]byte(browserLoginCompletionPage("AFS login failed", "The login response did not match this CLI session. Close this tab and try again.", false)))
			select {
			case resultCh <- browserLoginResult{Err: fmt.Errorf("browser login returned an unexpected state")}:
			default:
			}
			return
		}

		if message := strings.TrimSpace(query.Get("error")); message != "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(browserLoginCompletionPage("AFS login failed", message, false)))
			select {
			case resultCh <- browserLoginResult{Err: errors.New(message)}:
			default:
			}
			return
		}

		token := strings.TrimSpace(query.Get("token"))
		if token == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(browserLoginCompletionPage("AFS login failed", "No onboarding token was returned to the CLI.", false)))
			select {
			case resultCh <- browserLoginResult{Err: fmt.Errorf("browser login returned no onboarding token")}:
			default:
			}
			return
		}

		_, _ = w.Write([]byte(browserLoginCompletionPage("AFS login complete", "You can return to the terminal. The CLI will finish connecting automatically.", true)))
		select {
		case resultCh <- browserLoginResult{Token: token}:
		default:
		}
	})
	return mux
}

func browserLoginCompletionPage(title, message string, success bool) string {
	iconSVG := `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.25" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="10"></circle><path d="M15 9l-6 6"></path><path d="M9 9l6 6"></path></svg>`
	if success {
		iconSVG = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.25" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><polyline points="20 6 9 17 4 12"></polyline></svg>`
	}
	iconClass := "icon"
	if !success {
		iconClass = "icon icon--error"
	}
	return `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>` + html.EscapeString(title) + `</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Nunito+Sans:wght@400;600;700&display=swap" rel="stylesheet">
    <style>
      :root {
        color-scheme: light dark;
        --afs-bg: #edf1f7;
        --afs-panel: #ffffff;
        --afs-line: rgba(8, 6, 13, 0.08);
        --afs-ink: #08060d;
        --afs-muted: #626b78;
        --afs-accent: #2563eb;
        --afs-accent-soft: rgba(37, 99, 235, 0.1);
        --afs-danger: #dc2626;
        --afs-danger-soft: rgba(220, 38, 38, 0.1);
        --afs-shadow: 0 24px 70px rgba(8, 6, 13, 0.08);
      }
      @media (prefers-color-scheme: dark) {
        :root {
          --afs-bg: #0b1b24;
          --afs-panel: #122733;
          --afs-line: rgba(58, 92, 110, 0.62);
          --afs-ink: #f2f5f4;
          --afs-muted: #98aab1;
          --afs-accent: #60a5fa;
          --afs-accent-soft: rgba(96, 165, 250, 0.16);
          --afs-danger: #f87171;
          --afs-danger-soft: rgba(248, 113, 113, 0.18);
          --afs-shadow: 0 24px 70px rgba(0, 0, 0, 0.3);
        }
      }
      * { box-sizing: border-box; }
      html, body { min-height: 100%; }
      body {
        margin: 0;
        font-family: "Nunito Sans", -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
        background: var(--afs-bg);
        color: var(--afs-ink);
        display: flex;
        align-items: center;
        justify-content: center;
        padding: 32px 24px;
      }
      @media (prefers-color-scheme: dark) {
        body {
          background: linear-gradient(180deg, #0f2230 0%, #0b1b24 16%, #0b1b24 100%);
        }
      }
      .card {
        width: 100%;
        max-width: 480px;
        background: var(--afs-panel);
        border: 1px solid var(--afs-line);
        border-radius: 20px;
        padding: 36px 32px 32px;
        box-shadow: var(--afs-shadow);
        text-align: center;
      }
      .icon {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        width: 56px;
        height: 56px;
        border-radius: 50%;
        background: var(--afs-accent-soft);
        color: var(--afs-accent);
        margin-bottom: 20px;
      }
      .icon--error {
        background: var(--afs-danger-soft);
        color: var(--afs-danger);
      }
      .icon svg { width: 28px; height: 28px; }
      h1 {
        margin: 0 0 10px;
        font-size: 26px;
        font-weight: 700;
        line-height: 1.2;
        letter-spacing: -0.015em;
        color: var(--afs-ink);
      }
      p {
        margin: 0;
        font-size: 15px;
        line-height: 1.6;
        color: var(--afs-muted);
      }
    </style>
  </head>
  <body>
    <div class="card">
      <div class="` + iconClass + `">` + iconSVG + `</div>
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
