package cli

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// errExtraJSONValue is a body that holds more than one JSON value.
var errExtraJSONValue = errors.New("the body holds more than one JSON value")

// decodeOneJSON reads exactly one JSON value from the request body, at
// most limit bytes, and refuses fields T does not have. A body over limit
// fails with *http.MaxBytesError while the first value is read, and with
// errExtraJSONValue when anything follows it.
func decodeOneJSON[T any](w http.ResponseWriter, r *http.Request, limit int64) (T, error) {
	var v T
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return v, err
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		return v, errExtraJSONValue
	}
	return v, nil
}
