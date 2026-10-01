package cli

import (
	"reflect"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

// A cleaned value takes over the earlier claim only while that claim
// still has the sum the cleanup started from (#1556).
func TestFollowedKeys_MoveTheClaimOnlyFromAnUneditedValue(t *testing.T) {
	cleaned := adapters.MergedKey{Path: []string{"hooks"}, Sum: "after", Follows: "before"}
	plain := adapters.MergedKey{Path: []string{"model"}, Sum: "m"}
	for name, c := range map[string]struct {
		last []adapters.MergedKey
		want []adapters.MergedKey
	}{
		"unedited":      {[]adapters.MergedKey{{Path: []string{"hooks"}, Sum: "before"}}, []adapters.MergedKey{plain, {Path: []string{"hooks"}, Sum: "after"}}},
		"edited":        {[]adapters.MergedKey{{Path: []string{"hooks"}, Sum: "other"}}, []adapters.MergedKey{plain}},
		"never claimed": {nil, []adapters.MergedKey{plain}},
		"item claim":    {[]adapters.MergedKey{{Path: []string{"hooks"}, Items: []string{"x"}}}, []adapters.MergedKey{plain}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := followedKeys([]adapters.MergedKey{plain, cleaned}, c.last); !reflect.DeepEqual(got, c.want) {
				t.Errorf("followedKeys = %+v, want %+v", got, c.want)
			}
		})
	}
}
