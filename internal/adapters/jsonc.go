package adapters

import "github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"

// StripJSONC removes comments and trailing commas before JSON decoding.
// It preserves byte offsets and reports whether comments were present.
func StripJSONC(data []byte) ([]byte, bool) {
	return emit.StripJSONC(data)
}
