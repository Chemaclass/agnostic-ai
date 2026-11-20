package cli

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// errApplyWithoutPreview answers an apply that carries no expected value.
const errApplyWithoutPreview = "apply needs the expected value a preview returned; send action preview first"

// errExtraJSONValue is a body that holds more than one JSON value.
var errExtraJSONValue = errors.New("the body holds more than one JSON value")

// decodeOneJSON reads exactly one JSON value from the request body into
// v, at most limit bytes, and refuses fields v does not have. A body over
// limit fails with *http.MaxBytesError while the first value is read, and
// with errExtraJSONValue when anything follows it.
func decodeOneJSON(w http.ResponseWriter, r *http.Request, limit int64, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errExtraJSONValue
	}
	return nil
}
