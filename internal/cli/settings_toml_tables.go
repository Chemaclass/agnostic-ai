package cli

import (
	"fmt"
	"slices"
	"strings"
)

const tableTOMLFormat = "table-toml"
const tomlTablesContainer = "\x00toml-table-separators"

func nestedTOMLFormat(format string) bool {
	return format == grokTOMLFormat || format == tableTOMLFormat
}

func settingsTableContainer(format string) string {
	switch format {
	case grokTOMLFormat:
		return grokModelsContainer
	case tableTOMLFormat:
		return tomlTablesContainer
	default:
		return ""
	}
}

func settingsOwnsTable(owned map[string]any, format string) bool {
	if format == grokTOMLFormat {
		return grokOwnsTable(owned)
	}
	for key := range owned {
		if key != tomlTablesContainer && strings.Contains(key, ".") {
			return true
		}
	}
	return false
}

func editSettingsTOMLTables(path string, data []byte, order []string, set map[string]any, remove []string, previous map[string]any) ([]byte, any, error) {
	containers := grokTableSeparators(previous[tomlTablesContainer])
	var tables []string
	for _, key := range slices.Concat(order, remove) {
		at := strings.LastIndexByte(key, '.')
		if at <= 0 || at == len(key)-1 {
			return nil, nil, fmt.Errorf("%s: %s must name a table and a scalar member", path, key)
		}
		if !slices.Contains(tables, key[:at]) {
			tables = append(tables, key[:at])
		}
	}
	for _, table := range tables {
		local := map[string]any{}
		var localOrder, localRemove []string
		for _, key := range order {
			if member, ok := strings.CutPrefix(key, table+"."); ok {
				local[member] = set[key]
				localOrder = append(localOrder, member)
			}
		}
		for _, key := range remove {
			if member, ok := strings.CutPrefix(key, table+"."); ok {
				localRemove = append(localRemove, member)
			}
		}
		var err error
		data, err = editGrokTable(path, data, table, localOrder, local, localRemove, false, false, containers)
		if err != nil {
			return nil, nil, err
		}
	}
	if len(containers) == 0 {
		return data, nil, nil
	}
	return data, containers, nil
}
