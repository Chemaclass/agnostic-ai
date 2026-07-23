package emit

import (
	"fmt"
	"os"
)

// DiskState classifies how a candidate file's content relates to what is
// currently on disk at its path. It is the verdict returned by CompareToDisk
// and mirrors the buckets `sync --check` and `doctor` report: in-sync,
// missing, or stale (a local edit sync would overwrite).
type DiskState int

const (
	// DiskStale is the zero value on purpose. A dropped or unchecked verdict
	// must surface as drift, never as a false "in sync": reporting a dirty
	// tree as clean is the one failure mode drift detection may not have. It
	// means the file exists but its bytes differ from the candidate content.
	DiskStale DiskState = iota
	// DiskInSync means the file exists and its bytes equal the candidate.
	DiskInSync
	// DiskMissing means no file exists at the path yet.
	DiskMissing
)

// String renders the verdict for test output and diagnostics.
func (s DiskState) String() string {
	switch s {
	case DiskInSync:
		return "in-sync"
	case DiskMissing:
		return "missing"
	case DiskStale:
		return "stale"
	default:
		return fmt.Sprintf("DiskState(%d)", int(s))
	}
}

// CompareToDisk reports how content compares to the file already at path,
// classifying it as in-sync, missing, or stale. It is the capture-compare
// primitive behind drift detection: a captured would-be output is diffed
// against the artifact currently on disk.
//
// It short-circuits on size. A regular file whose byte size differs from
// len(content) is provably stale, so its body is never read. The cheap stat
// alone settles the verdict. Size is a negative signal only: equal size is
// necessary but not sufficient for equality, so a same-size hand edit still
// triggers a full byte compare and is reported stale. The verdict is
// therefore identical to an unconditional full read; only the I/O differs.
func CompareToDisk(path, content string) (DiskState, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DiskMissing, nil
		}
		return DiskStale, fmt.Errorf("stat %s: %w", path, err)
	}
	// Fast path: a size mismatch on a regular file proves drift without
	// reading the body. Irregular entries (a directory shadowing the path,
	// say) fall through to the read below so their error handling matches an
	// unconditional os.ReadFile rather than being silently classified.
	if info.Mode().IsRegular() && info.Size() != int64(len(content)) {
		return DiskStale, nil
	}
	disk, err := os.ReadFile(path)
	if err != nil {
		// The file vanished between the stat and the read (a concurrent sync,
		// say). Treat it as missing, exactly as a plain os.ReadFile of an
		// absent file would be classified.
		if os.IsNotExist(err) {
			return DiskMissing, nil
		}
		return DiskStale, fmt.Errorf("read %s: %w", path, err)
	}
	if string(disk) != content {
		return DiskStale, nil
	}
	return DiskInSync, nil
}
