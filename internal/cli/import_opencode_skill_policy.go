package cli

import "gopkg.in/yaml.v3"

func preserveOpencodeSkillPolicy(existing, imported *yaml.Node) {
	index := mappingIndex(existing, "x-opencode")
	if index < 0 {
		return
	}
	original := existing.Content[index+1]
	var values map[string]any
	if original.Decode(&values) != nil || values == nil {
		return
	}
	custom := &yaml.Node{}
	if custom.Encode(values) != nil {
		return
	}
	detachDependents(existing, original)
	removeMappingKey(custom, "disable-model-invocation")
	metadataIndex := mappingIndex(custom, "metadata")
	if metadataIndex >= 0 {
		metadata := custom.Content[metadataIndex+1]
		if metadata.Kind == yaml.MappingNode {
			removeMappingKey(metadata, "opencode/autoinvoke")
			_, native := topLevelPair(imported, "metadata")
			if native != nil {
				native = detachImportOverride(native)
				if native.Kind == yaml.MappingNode {
					for i := 0; i+1 < len(native.Content); i += 2 {
						key, value := native.Content[i], native.Content[i+1]
						prior := mappingIndex(metadata, key.Value)
						if prior < 0 {
							metadata.Content = append(metadata.Content, key, value)
						} else {
							metadata.Content[prior+1] = value
						}
					}
				}
			}
			if len(metadata.Content) == 0 {
				removeMappingKey(custom, "metadata")
			}
		} else {
			removeMappingKey(custom, "metadata")
		}
	}
	if len(custom.Content) == 0 {
		removeMappingKey(existing, "x-opencode")
	} else {
		existing.Content[index+1] = custom
	}
}

func detachDependents(existing, replaced *yaml.Node) {
	removed := map[*yaml.Node]bool{}
	var collect func(*yaml.Node)
	collect = func(node *yaml.Node) {
		removed[node] = true
		for _, child := range node.Content {
			collect(child)
		}
	}
	collect(replaced)
	var detach func(*yaml.Node)
	detach = func(node *yaml.Node) {
		for i, child := range node.Content {
			if child.Kind == yaml.AliasNode && removed[child.Alias] {
				node.Content[i] = detachImportOverride(child)
			} else {
				detach(child)
			}
		}
	}
	detach(existing)
}
