package mcpprobe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Headers runs a configured shell helper within the probe's deadline.
func Headers(ctx context.Context, helper, cwd string) (headers map[string]string, err error) {
	cmd := headersCommand(ctx, helper)
	cmd.Dir = cwd
	cmd.WaitDelay = 100 * time.Millisecond
	var out boundedHeaders
	cmd.Stdout = &out
	cleanup, err := startProcess(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("headersHelper failed: %w", err)
	}
	defer func() { err = errors.Join(err, cleanup()) }()
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("headersHelper failed: %w", ctx.Err())
		}
		return nil, fmt.Errorf("headersHelper failed: %w", err)
	}
	if err := json.Unmarshal(out.Bytes(), &headers); err != nil || headers == nil {
		return nil, errors.New("headersHelper did not print a JSON object of headers")
	}
	return headers, nil
}

type boundedHeaders struct{ bytes.Buffer }

func (b *boundedHeaders) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 1<<20 {
		return 0, errors.New("headersHelper output exceeds 1 MiB")
	}
	return b.Buffer.Write(p)
}
