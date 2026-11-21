package conversations

import (
	"encoding/binary"
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

// environmentReader on macOS reads each process's kern.procargs2, the
// arguments and environment the kernel keeps for it, which ps -E prints
// on one line with no way to tell where an entry ends.
var environmentReader environmentRead = procargsEnvironments

func procargsEnvironments(pids []int) (map[int][]string, error) {
	environments := map[int][]string{}
	for _, pid := range pids {
		raw, err := unix.SysctlRaw("kern.procargs2", pid)
		if err != nil {
			// A process that ended since its lock was read has no
			// environment, and runs nothing to link.
			if errors.Is(unix.Kill(pid, 0), unix.ESRCH) {
				continue
			}
			return nil, fmt.Errorf("kern.procargs2 of process %d: %w", pid, err)
		}
		entries, ok := procargsEnvironment(raw)
		if !ok {
			continue // Its boundaries cannot be read, so it links nothing.
		}
		environments[pid] = keptEntries(entries)
	}
	return environments, nil
}

// procargsEnvironment reads the environment out of a kern.procargs2
// value: the argument count as a 32-bit integer, the executable path,
// the NUL bytes that pad it, that many arguments, then the environment
// entries up to an empty one. Every string ends with a NUL byte, so an
// argument or a value with spaces stays one entry. ok is false when the
// value ends before its arguments do: then nothing tells an argument
// from an entry.
func procargsEnvironment(raw []byte) (entries []string, ok bool) {
	if len(raw) < 4 {
		return nil, false
	}
	argc := int(binary.NativeEndian.Uint32(raw[:4]))
	rest := raw[4:]
	path, rest, found := cutNUL(rest)
	if !found || path == "" {
		return nil, false
	}
	for len(rest) > 0 && rest[0] == 0 {
		rest = rest[1:]
	}
	for range argc {
		_, rest, found = cutNUL(rest)
		if !found {
			return nil, false
		}
	}
	for {
		entry, after, found := cutNUL(rest)
		if !found || entry == "" {
			return entries, true
		}
		entries = append(entries, entry)
		rest = after
	}
}

// cutNUL splits raw at its first NUL byte.
func cutNUL(raw []byte) (before string, after []byte, found bool) {
	for i, b := range raw {
		if b == 0 {
			return string(raw[:i]), raw[i+1:], true
		}
	}
	return "", nil, false
}
