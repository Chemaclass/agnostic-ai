//go:build !unix

package cli

import "os"

func settingsFileShared(os.FileInfo) string {
	return ""
}
