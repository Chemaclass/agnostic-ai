//go:build !darwin || !cgo

package main

import "unsafe"

// watchPointer does nothing here: the web view's own hover is enough
// outside macOS.
func watchPointer(unsafe.Pointer, func(over bool)) {}
