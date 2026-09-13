package emit

// WithFactoryMCPExtras enables Factory's documented per-server controls.
// Its OAuth object differs from Claude's, so the mapping stays separate.
func WithFactoryMCPExtras() MCPOption {
	return func(o *mcpOptions) { o.factoryExtras = true }
}

func addFactoryMCPFields(out, meta map[string]any, transport string) {
	for _, key := range []string{"timeout", "connectTimeout"} {
		if value, ok := IntField(meta, key); ok {
			out[key] = value
		}
	}
	if tools, ok := scopeList(meta, "disabledTools"); ok {
		out["disabledTools"] = tools
	}
	if transport != "http" && transport != "sse" {
		return
	}
	if enabled, ok := meta["oauth"].(bool); ok && !enabled {
		out["oauth"] = false
		return
	}
	raw, ok := meta["oauth"].(map[string]any)
	if !ok {
		return
	}
	oauth := map[string]any{}
	for _, key := range []string{"scopes", "resource", "authorizationServerIssuer", "clientId", "clientSecret", "clientMetadataUrl", "tokenEndpointAuthMethod", "callbackPort"} {
		if value, present := raw[key]; present {
			oauth[key] = value
		}
	}
	if len(oauth) > 0 {
		out["oauth"] = oauth
	}
}
