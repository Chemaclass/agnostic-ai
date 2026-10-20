package spec

type LayerCandidate struct {
	Entry     Entry
	Operation string
	Role      string
}

type LayerHistory []LayerCandidate

func LoadLayeredWithHistory(layers []Layer) (Bundle, LayerHistory, error) {
	recorder := layerHistoryRecorder{active: make(map[layerHistoryKey][]int)}
	bundle, err := loadLayered(layers, &recorder)
	if err != nil {
		return Bundle{}, nil, err
	}
	return bundle, recorder.candidates, nil
}

type layerHistoryKey struct {
	kind Kind
	name string
}

type layerHistoryRecorder struct {
	candidates LayerHistory
	active     map[layerHistoryKey][]int
}

func (r *layerHistoryRecorder) record(entry Entry, operation string) {
	key := layerHistoryKey{kind: entry.Kind, name: entry.Name}
	if operation == "replace" {
		for _, i := range r.active[key] {
			r.candidates[i].Role = "replaced"
		}
		r.active[key] = nil
	}
	r.active[key] = append(r.active[key], len(r.candidates))
	r.candidates = append(r.candidates, LayerCandidate{
		Entry: entry, Operation: operation, Role: "contributor",
	})
}
