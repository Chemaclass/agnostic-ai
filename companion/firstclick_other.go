//go:build !darwin || !cgo

package main

// takeFirstClick does nothing here: only macOS spends the first click
// on a window of another app on bringing it forward.
func takeFirstClick() {}
