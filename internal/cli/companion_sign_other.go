//go:build !darwin

package cli

import (
	"context"
	"errors"
)

func signCompanion(context.Context, string, string) error {
	return errors.New("only macOS signs the companion app")
}
