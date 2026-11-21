//go:build !darwin

package main

import "runtime"

func nativeUpdateArch() string {
	return runtime.GOARCH
}
