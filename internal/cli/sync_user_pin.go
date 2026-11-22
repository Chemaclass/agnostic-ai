package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// userSyncPin binds a user sync to one plan: a run under it records the
// fingerprint of what it would write, and a run that wants a fingerprint
// writes nothing when the plan moved.
type userSyncPin struct{ want, got string }

type userSyncPinKey struct{}

// errUserSyncMoved stops a pinned apply whose writes or removals differ
// from the plan the person confirmed.
var errUserSyncMoved = errors.New("the pending changes changed since you checked them")

func withUserSyncPin(ctx context.Context, pin *userSyncPin) context.Context {
	return context.WithValue(ctx, userSyncPinKey{}, pin)
}

// checkPin records run's fingerprint on the pin in ctx, if any, and
// refuses a run that differs from the one it wants.
func (r userRun) checkPin(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	pin, ok := ctx.Value(userSyncPinKey{}).(*userSyncPin)
	if !ok {
		return nil
	}
	pin.got = r.fingerprint()
	if pin.want != "" && pin.got != pin.want {
		return errUserSyncMoved
	}
	return nil
}

// fingerprint sums every planned write (path, mode and content) and
// every removal, so the same plan id means the same changes.
func (r userRun) fingerprint() string {
	lines := make([]string, 0, len(r.writes)+len(r.removals))
	for _, w := range r.writes {
		sum := sha256.Sum256(w.data)
		lines = append(lines, fmt.Sprintf("write\x00%s\x00%o\x00%x", w.path, w.mode, sum))
	}
	for _, path := range r.removals {
		lines = append(lines, "remove\x00"+path)
	}
	slices.Sort(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}
