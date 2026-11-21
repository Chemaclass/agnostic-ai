//go:build !linux || !cgo || gtk3 || android || server || purego

package main

// seeThrough leaves the windows to the window library here.
func seeThrough() {}
