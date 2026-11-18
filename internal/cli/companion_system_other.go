//go:build !darwin

package cli

import "runtime"

func macOSVersion() string { return "" }

func machineArch() string { return runtime.GOARCH }
