package emit

// Refusal is an error a target raises before writing anything, for a spec
// or setting it cannot accept. Path is the spec it names, empty when the
// project config is the cause.
type Refusal struct {
	Path string
	Err  error
}

func (r *Refusal) Error() string {
	if r.Path == "" {
		return r.Err.Error()
	}
	return r.Path + ": " + r.Err.Error()
}

func (r *Refusal) Unwrap() error { return r.Err }
