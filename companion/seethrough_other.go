//go:build !linux || !cgo

package main

// seeThrough does nothing here: the window library makes the windows
// see-through by itself.
func seeThrough() {}
