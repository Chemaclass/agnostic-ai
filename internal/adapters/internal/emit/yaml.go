package emit

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// YAMLScalar renders s as a YAML scalar, quoting it only when YAML
// needs it (a leading `*`, a leading `#`, ...). Always double-quoting
// instead rewrote `apps/foo/**` to `"apps/foo/**"` and broke byte
// round-trips against hand-authored frontmatter (#443).
func YAMLScalar(s string) string {
	out, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Sprintf("%q", s)
	}
	return strings.TrimRight(string(out), "\n")
}
