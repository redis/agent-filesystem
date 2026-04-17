package main

import (
	"os"
	"strings"
)

const controlPlaneConfigPathEnvVar = "AFS_CONFIG_PATH"

func defaultListenAddr() string {
	if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
		return ":" + port
	}
	return "127.0.0.1:8091"
}

func defaultConfigPath() string {
	return strings.TrimSpace(os.Getenv(controlPlaneConfigPathEnvVar))
}
