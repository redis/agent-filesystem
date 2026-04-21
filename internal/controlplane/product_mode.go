package controlplane

import (
	"os"
	"strings"
)

const (
	// ProductModeEnvVar declares whether this control plane is the Anthropic-
	// hosted cloud build or a self-hosted deployment. Affects install.sh output
	// and the /v1/auth/config response so the UI can tailor onboarding.
	ProductModeEnvVar = "AFS_PRODUCT_MODE"

	// SeedGettingStartedEnvVar controls whether a fresh self-hosted control
	// plane auto-seeds a `getting-started` workspace on first boot. Defaults
	// to "1" in self-hosted mode, "0" in cloud. Operators can override either
	// way by setting the variable explicitly.
	SeedGettingStartedEnvVar = "AFS_SEED_GETTING_STARTED"

	ProductModeCloud      = "cloud"
	ProductModeSelfHosted = "self-hosted"
)

// ProductModeFromEnv returns the declared product mode. Defaults to
// "self-hosted" — the cloud deployment must opt in explicitly.
func ProductModeFromEnv() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(ProductModeEnvVar))) {
	case ProductModeCloud:
		return ProductModeCloud
	default:
		return ProductModeSelfHosted
	}
}

// ShouldSeedGettingStarted reports whether boot-time seeding of a
// `getting-started` workspace is enabled. An explicit value of the env var
// takes precedence; otherwise defaults to true for self-hosted mode and
// false for cloud.
func ShouldSeedGettingStarted() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(SeedGettingStartedEnvVar))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return ProductModeFromEnv() == ProductModeSelfHosted
}
