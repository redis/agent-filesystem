package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const defaultCloudControlPlaneURL = "https://agentfilesystem.ai"

type authExchangeResponse struct {
	DatabaseID    string `json:"database_id"`
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
	AccessToken   string `json:"access_token,omitempty"`
}

var runBrowserLoginFlow = launchBrowserLoginFlow

func cmdOnboard(args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, onboardUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	return cmdAuthLogin(args)
}

func cmdAuth(args []string) error {
	if len(args) < 2 {
		return cmdAuthLogin(nil)
	}
	if isHelpArg(args[1]) {
		printAuthUsage()
		return nil
	}
	if strings.HasPrefix(args[1], "-") {
		return cmdAuthLogin(args[1:])
	}

	switch args[1] {
	case "login":
		return cmdAuthLogin(args[2:])
	case "logout":
		return cmdAuthLogout(args[2:])
	case "status":
		return cmdAuthStatus(args[2:])
	default:
		return fmt.Errorf("unknown auth subcommand %q\n\n%s", args[1], authUsageText(filepath.Base(os.Args[0])))
	}
}

func cmdAuthLogin(args []string) error {
	fs := flag.NewFlagSet("auth login", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var controlPlaneURL optionalString
	var token optionalString
	var workspace optionalString
	fs.Var(&controlPlaneURL, "control-plane-url", "http:// or https:// hosted control plane URL")
	fs.Var(&token, "token", "one-time onboarding token from the control plane")
	fs.Var(&workspace, "workspace", "preferred workspace id or name for browser login")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%s", authLoginUsageText(filepath.Base(os.Args[0])))
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("%s", authLoginUsageText(filepath.Base(os.Args[0])))
	}

	cfg := loadConfigOrDefault()
	baseURL := strings.TrimSpace(controlPlaneURL.value)
	if baseURL == "" {
		baseURL = strings.TrimSpace(cfg.URL)
	}
	if baseURL == "" {
		baseURL = defaultCloudControlPlaneURL
	}
	normalizedURL, err := normalizeControlPlaneURL(baseURL)
	if err != nil {
		return err
	}
	resolvedToken := strings.TrimSpace(token.value)
	if resolvedToken == "" {
		resolvedToken, err = runBrowserLoginFlow(normalizedURL, strings.TrimSpace(workspace.value))
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
	cfg.Mode = modeSync

	if err := resolveConfigPaths(&cfg); err != nil {
		return err
	}
	if err := saveConfig(cfg); err != nil {
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
		fmt.Fprint(os.Stderr, authLogoutUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	if len(args) != 0 {
		return fmt.Errorf("%s", authLogoutUsageText(filepath.Base(os.Args[0])))
	}

	cfg, err := loadConfig()
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no configuration found\nRun '%s setup' first", filepath.Base(os.Args[0]))
		}
		return err
	}

	cfg.ProductMode = productModeLocal
	cfg.URL = ""
	cfg.DatabaseID = ""
	cfg.AuthToken = ""
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

func cmdAuthStatus(args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stderr, authStatusUsageText(filepath.Base(os.Args[0])))
		return nil
	}
	if len(args) != 0 {
		return fmt.Errorf("%s", authStatusUsageText(filepath.Base(os.Args[0])))
	}

	cfg, hasSavedConfig, err := loadConfigWithPresence()
	if err != nil {
		return err
	}
	if !hasSavedConfig {
		printBox(clr(ansiBold, "auth"), []boxRow{
			{Label: "status", Value: "not signed in"},
			{Label: "hint", Value: clr(ansiDim, "Run '"+filepath.Base(os.Args[0])+" onboard'")},
		})
		return nil
	}
	if err := prepareConfigForSave(&cfg); err != nil {
		return err
	}

	productMode, err := effectiveProductMode(cfg)
	if err != nil {
		return err
	}
	if productMode != productModeCloud {
		printBox(clr(ansiBold, "auth"), []boxRow{
			{Label: "status", Value: "not signed in to cloud"},
			{Label: "mode", Value: productModeDisplayLabel(productMode)},
		})
		return nil
	}
	if strings.TrimSpace(cfg.AuthToken) == "" {
		printBox(clr(ansiBold, "auth"), []boxRow{
			{Label: "status", Value: "cloud login needs refresh"},
			{Label: "control plane", Value: cfg.URL},
			{Label: "hint", Value: clr(ansiDim, "Run '"+filepath.Base(os.Args[0])+" onboard' again to finish browser sign-in.")},
		})
		return nil
	}

	rows := []boxRow{
		{Label: "status", Value: "signed in"},
		{Label: "control plane", Value: cfg.URL},
	}
	if strings.TrimSpace(cfg.CurrentWorkspace) != "" {
		rows = append(rows, boxRow{Label: "workspace", Value: cfg.CurrentWorkspace})
	}
	if strings.TrimSpace(cfg.DatabaseID) != "" {
		rows = append(rows, boxRow{Label: "database", Value: cfg.DatabaseID})
	}
	printBox(clr(ansiBold, "auth"), rows)
	return nil
}

func printAuthUsage() {
	fmt.Fprint(os.Stderr, authUsageText(filepath.Base(os.Args[0])))
}

func authUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s auth <command>
  %s auth [--control-plane-url <url>] [--workspace <workspace>]

Commands:
  login    Open the browser and connect this CLI to AFS Cloud
  logout   Clear the hosted cloud login from this machine
  status   Show current cloud login state

Shortcut:
  %s onboard  Preferred first-run browser onboarding flow
`, bin, bin, bin)
}

func authLoginUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s auth login --control-plane-url <url> --token <token>
  %s auth login [--control-plane-url <url>] [--workspace <workspace>]
`, bin, bin)
}

func onboardUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s onboard [--control-plane-url <url>] [--workspace <workspace>]

Examples:
  %s onboard
  %s onboard --workspace getting-started
`, bin, bin, bin)
}

func authLogoutUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s auth logout
`, bin)
}

func authStatusUsageText(bin string) string {
	return fmt.Sprintf(`Usage:
  %s auth status
`, bin)
}
