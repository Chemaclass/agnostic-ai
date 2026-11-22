//go:build !darwin && !linux && !windows

package main

import "os"

func tryLockFile(*os.File) error { return nil }

func lockHeldElsewhere(error) bool { return false }
