//go:build !unix

package cli

import "os"

const mailboxDeliveryReadFlags = os.O_RDONLY

func mailboxDeliveryOwned(os.FileInfo) bool { return false }
