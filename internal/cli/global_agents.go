package cli

import (
	"bytes"
	"fmt"
	"os"
	"slices"
	"sort"
)

// A partial sync must not change a shared agent still owned by another target.
// Select all owners together to compare their new native output before writing.
func checkUnselectedGlobalAgents(writes []globalWrite, old globalState, targets []string) error {
	var unselected []string
	for target := range old.Agents {
		if !slices.Contains(targets, target) {
			unselected = append(unselected, target)
		}
	}
	sort.Strings(unselected)
	for _, w := range writes {
		for _, target := range unselected {
			if !slices.Contains(old.Agents[target], w.path) {
				continue
			}
			data, err := os.ReadFile(w.path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return fmt.Errorf("read shared agent %s: %w", w.path, err)
			}
			if !bytes.Equal(data, w.data) {
				return fmt.Errorf("%s: shared agent is also owned by unselected target %s; include it in this sync to check for conflicting content", w.path, target)
			}
		}
	}
	return nil
}
