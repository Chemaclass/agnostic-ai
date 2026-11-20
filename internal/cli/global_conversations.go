package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
)

func loadGlobalConversations(source string) (*time.Duration, error) {
	var threshold *time.Duration
	for _, path := range globalConfigPaths(source) {
		doc, err := readGlobalConfig(path)
		if err != nil {
			return nil, err
		}
		node, ok := doc["conversations"]
		if !ok {
			continue
		}
		var layer config.ConversationsConfig
		if err := node.Decode(&layer); err != nil {
			return nil, errs.Coded(errs.CodeConfigDecode, "parse %s: conversations: %w", path, err)
		}
		after, err := layer.ShellThreshold(path)
		if err != nil {
			return nil, err
		}
		if after != nil {
			threshold = after
		}
	}
	return threshold, nil
}

func servedShellThreshold(warn io.Writer) *time.Duration {
	source, err := globalSourceRoot()
	if err == nil {
		var threshold *time.Duration
		threshold, err = loadGlobalConversations(source)
		if err == nil {
			return threshold
		}
	}
	_, _ = fmt.Fprintf(warn, "warning: %v; disabling the stuck shell warning and stop action\n", err)
	return new(time.Duration)
}
