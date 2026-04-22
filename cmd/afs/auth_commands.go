package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultCloudControlPlaneURL      = "https://agentfilesystem.ai"
	defaultSelfHostedControlPlaneURL = "http://127.0.0.1:8091"
)

type authExchangeResponse struct {
	DatabaseID    string `json:"database_id"`
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
	AccessToken   string `json:"access_token,omitempty"`
	Account       string `json:"account,omitempty"`
}

var runBrowserLoginFlow = launchBrowserLoginFlow

// cmdLogout clears any cached cloud login and flips the product mode back
// to local-only.
func cmdLogout(args []string) error {
	return cmdAuthLogout(args)
}

// cmdLogin connects the CLI to a control plane. It handles both AFS Cloud
// (browser flow + token exchange) and self-hosted deployments (URL prompt,
// no auth token). Choice of mode:
//
//   - --cloud         → force cloud
//   - --self-hosted   → force self-hosted (optional --url, default 127.0.0.1:8091)
//   - neither         → reuse prior config; if empty, prompt interactively
func cmdLogin(args []string) error {
	for _, a := range args {
		if isHelpArg(a) {
			fmt.Fprint(os.Stderr, loginUsageText(filepath.Base(os.Args[0])))
			return nil
		}
	}

	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var controlPlaneURL optionalString
	var token optionalString
	var workspace optionalString
	var cloud bool
	var selfHosted bool
	fs.Var(&controlPlaneURL, "control-plane-url", "http:// or https:// hosted control plane URL")
	fs.Var(&controlPlaneURL, "url", "alias for --control-plane-url")
	fs.Var(&token, "token", "one-time onboarding token from the control plane")
	fs.Var(&workspace, "workspace", "preferred workspace id or name for browser login")
	fs.BoolVar(&cloud, "cloud", false, "force cloud mode (browser OAuth)")
	fs.BoolVar(&selfHosted, "self-hosted", false, "force self-hosted mode (URL-only)")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%s", loginUsageText(filepath.Base(os.Args[0])))
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("%s", loginUsageText(filepath.Base(os.Args[0])))
	}
	if cloud && selfHosted {
		return fmt.Errorf("--cloud and --self-hosted are mutually exclusive")
	}

	cfg := loadConfigOrDefault()
	mode, err := resolveLoginMode(cfg, cloud, selfHosted, controlPlaneURL.value, token.value)
	if err != nil {
		return err
	}

	switch mode {
	case productModeSelfHosted:
		return runSelfHostedLogin(&cfg, controlPlaneURL.value)
	case productModeCloud:
		return runCloudLogin(&cfg, controlPlaneURL.value, token.value, workspace.value)
	default:
		return fmt.Errorf("unsupported login mode %q", mode)
	}
}

// resolveLoginMode picks cloud vs self-hosted based on flags, then prior
// config, then falls back to an interactive prompt on stdin.
func resolveLoginMode(cfg config, cloud, selfHosted bool, overrideURL, overrideToken string) (string, error) {
	if cloud {
		return productModeCloud, nil
	}
	if selfHosted {
		return productModeSelfHosted, nil
	}
	// An explicit onboarding token is a cloud-flow signal — self-hosted has
	// no token exchange. This covers the common test/CI path where the URL
	// happens to be localhost but the caller is exercising the cloud flow.
	if strings.TrimSpace(overrideToken) != "" {
		return productModeCloud, nil
	}
	if strings.TrimSpace(overrideURL) != "" && looksLikeSelfHostedURL(overrideURL) {
		return productModeSelfHosted, nil
	}
	switch strings.TrimSpace(cfg.ProductMode) {
	case productModeSelfHosted:
		return productModeSelfHosted, nil
	case productModeCloud:
		return productModeCloud, nil
	}
	return promptLoginMode()
}

// looksLikeSelfHostedURL returns true for URLs that are clearly not the AFS
// cloud host. Used to infer `--self-hosted` when the user passed `--url`
// without a mode flag.
func looksLikeSelfHostedURL(raw string) bool {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return false
	}
	return !strings.Contains(raw, "agentfilesystem.ai") && !strings.Contains(raw, "agentfilesystem.vercel.app")
}

func promptLoginMode() (string, error) {
	r := bufio.NewReader(os.Stdin)
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "  "+clr(ansiBold+ansiCyan, "▸")+" "+clr(ansiBold, "Connect to a control plane"))
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "    "+clr(ansiCyan, "1")+"  "+clr(ansiBold, "Cloud")+"        "+clr(ansiDim, "— sign in to AFS Cloud via browser"))
	fmt.Fprintln(os.Stdout, "    "+clr(ansiCyan, "2")+"  "+clr(ansiBold, "Self-managed")+"  "+clr(ansiDim, "— point this CLI at your own control plane"))
	fmt.Fprintln(os.Stdout)
	choice, err := promptString(r, os.Stdout, "  Choose", "1")
	if err != nil {
		return "", err
	}
	switch strings.TrimSpace(strings.ToLower(choice)) {
	case "2", "self", "self-hosted", "selfhosted":
		return productModeSelfHosted, nil
	default:
		return productModeCloud, nil
	}
}

func runSelfHostedLogin(cfg *config, overrideURL string) error {
	baseURL := strings.TrimSpace(overrideURL)
	if baseURL == "" {
		// Prior cfg.URL only survives if it's already self-hosted — otherwise
		// it's likely a stale cloud URL we should not carry forward.
		prior := strings.TrimSpace(cfg.URL)
		if prior != "" && looksLikeSelfHostedURL(prior) {
			baseURL = prior
		}
	}
	if baseURL == "" {
		r := bufio.NewReader(os.Stdin)
		fmt.Fprintln(os.Stdout)
		entered, err := promptString(r, os.Stdout,
			"  Control plane URL\n  "+clr(ansiDim, "Example: "+defaultSelfHostedControlPlaneURL),
			defaultSelfHostedControlPlaneURL)
		if err != nil {
			return err
		}
		baseURL = entered
	}

	normalized, err := normalizeControlPlaneURL(baseURL)
	if err != nil {
		return err
	}

	// Verify the control plane is reachable before persisting anything.
	anon, err := newAnonymousHTTPControlPlaneClient(normalized)
	if err != nil {
		return err
	}
	if err := anon.Ping(context.Background()); err != nil {
		return fmt.Errorf("cannot reach control plane at %s: %w", normalized, err)
	}

	cfg.ProductMode = productModeSelfHosted
	cfg.URL = normalized
	// Self-hosted has no auth token. Reset any carried-over cloud state.
	cfg.AuthToken = ""
	if strings.TrimSpace(cfg.Mode) == "" {
		cfg.Mode = modeSync
	}

	if err := resolveConfigPaths(cfg); err != nil {
		return err
	}
	if err := saveConfig(*cfg); err != nil {
		return err
	}

	printBox(markerSuccess+" "+clr(ansiBold, "connected to self-managed control plane"), []boxRow{
		{Label: "control plane", Value: cfg.URL},
		{Label: "config", Value: clr(ansiDim, compactDisplayPath(configPath()))},
		{},
		{Label: "next", Value: clr(ansiOrange, filepath.Base(os.Args[0])+" setup") + clr(ansiDim, "   (pick a workspace)")},
	})
	return nil
}

func runCloudLogin(cfg *config, overrideURL, overrideToken, workspace string) error {
	baseURL := strings.TrimSpace(overrideURL)
	if baseURL == "" {
		prior := strings.TrimSpace(cfg.URL)
		if prior != "" && !looksLikeSelfHostedURL(prior) {
			baseURL = prior
		}
	}
	if baseURL == "" {
		baseURL = defaultCloudControlPlaneURL
	}
	normalizedURL, err := normalizeControlPlaneURL(baseURL)
	if err != nil {
		return err
	}

	resolvedToken := strings.TrimSpace(overrideToken)
	if resolvedToken == "" {
		resolvedToken, err = runBrowserLoginFlow(normalizedURL, strings.TrimSpace(workspace))
		if err != nil {
			return err
		}
	}

	client, err := newAnonymousHTTPControlPlaneClient(normalizedURL)
	if err != nil {
		return err
	}
	response, err := client.exchangeOnboardingToken(context.Background(), resolvedToken)
	if err != nil {
		return err
	}

	cfg.ProductMode = productModeCloud
	cfg.URL = normalizedURL
	cfg.DatabaseID = strings.TrimSpace(response.DatabaseID)
	cfg.CurrentWorkspaceID = strings.TrimSpace(response.WorkspaceID)
	cfg.CurrentWorkspace = strings.TrimSpace(response.WorkspaceName)
	cfg.AuthToken = strings.TrimSpace(response.AccessToken)
	cfg.Account = strings.TrimSpace(response.Account)
	cfg.Mode = modeSync

	if err := resolveConfigPaths(cfg); err != nil {
		return err
	}
	if err := saveConfig(*cfg); err != nil {
		return err
	}

	printBox(markerSuccess+" "+clr(ansiBold, "cloud login complete"), []boxRow{
		{Label: "control plane", Value: cfg.URL},
		{Label: "workspace", Value: cfg.CurrentWorkspace},
		{Label: "database", Value: cfg.DatabaseID},
		{},
		{Label: "next", Value: clr(ansiOrange, filepath.Base(os.Args[0])+" up")},
	})
	return nil
}

func cmdAuthLogout(args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, logoutUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	if len(args) != 0 {
		return fmt.Errorf("%s", logoutUsageText(filepath.Base(os.Args[0])))
	}

	cfg, err := loadConfig()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("no configuration found\nRun '%s login' first", filepath.Base(os.Args[0]))
		}
		return err
	}

	cfg.ProductMode = productModeLocal
	cfg.URL = ""
	cfg.DatabaseID = ""
	cfg.AuthToken = ""
	cfg.Account = ""
	cfg.CurrentWorkspace = ""
	cfg.CurrentWorkspaceID = ""

	if err := resolveConfigPaths(&cfg); err != nil {
		return err
	}
	if err := saveConfig(cfg); err != nil {
		return err
	}

	printBox(markerSuccess+" "+clr(ansiBold, "cloud login cleared"), []boxRow{
		{Label: "config", Value: clr(ansiDim, compactDisplayPath(configPath()))},
		{Label: "connection", Value: productModeDisplayLabel(productModeLocal)},
	})
	return nil
}

// authConnectionInfo summarises the current cloud-login state in a form that
// can be rendered as rows in `afs status`. Returns (rows, hasCloudConnection).
func authConnectionInfo(bin string) ([]boxRow, bool) {
	cfg, hasSavedConfig, err := loadConfigWithPresence()
	if err != nil {
		return []boxRow{{Label: "connection", Value: "error: " + err.Error()}}, false
	}
	if !hasSavedConfig {
		return []boxRow{
			{Label: "connection", Value: "not signed in"},
			{Label: "hint", Value: clr(ansiDim, "Run '"+bin+" login'")},
		}, false
	}
	if err := prepareConfigForSave(&cfg); err != nil {
		return []boxRow{{Label: "connection", Value: "error: " + err.Error()}}, false
	}
	productMode, err := effectiveProductMode(cfg)
	if err != nil {
		return []boxRow{{Label: "connection", Value: "error: " + err.Error()}}, false
	}
	if productMode == productModeLocal {
		return []boxRow{{Label: "connection", Value: productModeDisplayLabel(productMode)}}, false
	}
	rows := []boxRow{
		{Label: "connection", Value: productModeDisplayLabel(productMode)},
		{Label: "control plane", Value: cfg.URL},
	}
	if productMode == productModeCloud && strings.TrimSpace(cfg.AuthToken) == "" {
		rows = append(rows, boxRow{Label: "signed in", Value: "needs refresh"})
		rows = append(rows, boxRow{Label: "hint", Value: clr(ansiDim, "Run '"+bin+" login' again to finish browser sign-in.")})
		return rows, false
	}
	if productMode == productModeCloud {
		rows = append(rows, boxRow{Label: "signed in", Value: "yes"})
	}
	if db := strings.TrimSpace(cfg.DatabaseID); db != "" {
		rows = append(rows, boxRow{Label: "database", Value: db})
	}
	return rows, true
}

func loginUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s login [--cloud | --self-hosted [--url <url>]]
  %s login --control-plane-url <url> --token <token>

Flags:
  --cloud                   Force cloud mode (browser OAuth)
  --self-hosted             Force self-managed mode (URL-only)
  --url, --control-plane-url <url>
                            Override control plane URL (default %s for self-managed)
  --token <token>           One-time onboarding token (skips browser)
  --workspace <name|id>     Preferred workspace for cloud login

Examples:
  %s login
  %s login --self-hosted
  %s login --self-hosted --url http://my-host:8091
  %s login --cloud
`, bin, bin, defaultSelfHostedControlPlaneURL, bin, bin, bin, bin)
}

func logoutUsageText(bin string) string {
	return brandHeaderString() + fmt.Sprintf(`Usage:
  %s logout

Clears any cached cloud login from this machine and switches product mode
back to local-only. Safe to re-run when not signed in.
`, bin)
}
