//go:build !darwin || !cgo

package main

// clipboardHTML says nothing here, so the plain text goes as it is.
func clipboardHTML() string { return "" }
