package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type finalizationReceipt struct {
	ManifestSHA256 string `json:"manifest_sha256"`
	LockSHA256     string `json:"lock_sha256"`
}

func manifestDigest(m *manifest) string {
	b, _ := json.Marshal(m)
	return digest(b)
}

func writeFinalizationReceipt(m *manifest, lockHash string) error {
	b, err := json.Marshal(finalizationReceipt{ManifestSHA256: manifestDigest(m), LockSHA256: lockHash})
	if err != nil {
		return fmt.Errorf("encode finalization receipt: %w", err)
	}
	return atomicWrite(filepath.Join(m.Directory, "finalization.json"), append(b, '\n'))
}

func (a app) reconcileFinalization(m *manifest) (string, bool, error) {
	expected := m.Finalized
	if expected == "" {
		path := filepath.Join(m.Directory, "finalization.json")
		b, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		if err != nil {
			return "", true, fmt.Errorf("%s: %w", path, err)
		}
		var receipt finalizationReceipt
		decoder := json.NewDecoder(bytes.NewReader(b))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&receipt); err != nil {
			return "", true, fmt.Errorf("parse %s: %w", path, err)
		}
		if err = decoder.Decode(new(any)); err != io.EOF {
			return "", true, fmt.Errorf("%s: trailing receipt data", path)
		}
		if receipt.ManifestSHA256 != manifestDigest(m) || len(receipt.LockSHA256) != 64 {
			return "", true, fmt.Errorf("%s: finalization receipt does not match run", path)
		}
		current, err := a.lockDigest()
		if err != nil {
			return "", true, err
		}
		if current != receipt.LockSHA256 {
			if current == m.LockBaseline {
				return "", false, nil
			}
			return "", true, errors.New("interrupted finalization has a different current lock; reconcile before continuing")
		}
		expected = receipt.LockSHA256
	}
	if err := a.validateAtLock(m, expected); err != nil {
		return "", true, err
	}
	m.Finalized = expected
	if err := save(m); err != nil {
		return "", true, err
	}
	return "coverage validated: finalization already committed\n", true, nil
}
