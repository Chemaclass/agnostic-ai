package cli

import (
	"encoding/binary"
	"errors"
	"fmt"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// companionRunKey holds what starts at login, one value per program.
const companionRunKey = `Software\Microsoft\Windows\CurrentVersion\Run`

const companionRunApprovalKey = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`

// companionSysProcAttr starts the companion apart from this console, so
// closing the console does not end it.
func companionSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP}
}

// removeCompanionRunValue deletes the value that starts the companion
// at login, even one whose program is gone. It reports whether one was there.
func removeCompanionRunValue() (bool, error) {
	current, err := removeRunValue(registry.CURRENT_USER, companionRunKey, companionName)
	legacy, legacyErr := removeLegacyCompanionRunValue()
	return current || legacy, errors.Join(err, legacyErr)
}

func removeLegacyCompanionRunValue() (bool, error) {
	return removeRunValue(registry.CURRENT_USER, companionRunKey, legacyCompanionName)
}

func readLegacyCompanionRunValue() (companionLegacyRunValue, error) {
	return readLegacyRunValue(registry.CURRENT_USER, companionRunKey, companionRunApprovalKey)
}

func readLegacyRunValue(root registry.Key, path, approvalPath string) (companionLegacyRunValue, error) { //nolint:unparam // the registry hive stays explicit beside the key path it opens
	name := legacyCompanionName
	read := func(access uint32) (registry.Key, string, uint32, error) {
		key, err := registry.OpenKey(root, path, access|registry.QUERY_VALUE)
		if err != nil {
			return 0, "", 0, err
		}
		value, valueType, err := key.GetStringValue(name)
		return key, value, valueType, err
	}
	key, value, valueType, err := read(0)
	if key != 0 {
		_ = key.Close()
	}
	if errors.Is(err, registry.ErrNotExist) {
		return companionLegacyRunValue{}, nil
	}
	if err != nil {
		return companionLegacyRunValue{}, fmt.Errorf("read the earlier start at login value: %w", err)
	}
	enabled, known, err := legacyRunApproved(root, approvalPath)
	if err != nil {
		return companionLegacyRunValue{}, err
	}
	command := value
	if valueType == registry.EXPAND_SZ {
		if expanded, err := registry.ExpandString(value); err == nil {
			command = expanded
		}
	}
	return companionLegacyRunValue{
		program:         windowsCommandProgram(command),
		enabled:         enabled,
		unknownApproval: !known,
		remove: func() error {
			key, current, currentType, err := read(registry.SET_VALUE)
			if key != 0 {
				defer func() { _ = key.Close() }()
			}
			if err != nil {
				return err
			}
			if current != value || currentType != valueType {
				return errors.New("the earlier start at login value changed during install")
			}
			approved, _, err := legacyRunApproved(root, approvalPath)
			if err != nil {
				return err
			}
			if approved != enabled {
				return errors.New("Windows startup approval for the earlier companion changed during install")
			}
			return key.DeleteValue(name)
		},
		restore: func() error {
			key, _, err := registry.CreateKey(root, path, registry.QUERY_VALUE|registry.SET_VALUE)
			if err != nil {
				return err
			}
			defer func() { _ = key.Close() }()
			if _, _, err := key.GetStringValue(name); !errors.Is(err, registry.ErrNotExist) {
				return errors.New("the earlier start at login value was recreated during install")
			}
			if valueType == registry.EXPAND_SZ {
				return key.SetExpandStringValue(name, value)
			}
			return key.SetStringValue(name, value)
		},
	}, nil
}

// legacyRunApproved reports an unrecognized approval state as disabled and
// not known, so the install never turns on what the user may have turned off.
func legacyRunApproved(root registry.Key, path string) (enabled, known bool, err error) {
	key, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return true, true, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("read Windows startup approval under %s: %w", path, err)
	}
	defer func() { _ = key.Close() }()
	data, _, err := key.GetBinaryValue(legacyCompanionName)
	if errors.Is(err, registry.ErrNotExist) {
		return true, true, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("read Windows startup approval for %s: %w", legacyCompanionName, err)
	}
	// Explorer stores the approval state and an eight-byte timestamp.
	if len(data) == 12 {
		switch binary.LittleEndian.Uint32(data) {
		case 0, 2:
			return true, true, nil
		case 3:
			return false, true, nil
		}
	}
	return false, false, nil
}

func removeRunValue(root registry.Key, path, name string) (bool, error) { //nolint:unparam // the registry hive stays explicit beside the key path it opens
	key, err := registry.OpenKey(root, path, registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer func() { _ = key.Close() }()
	err = key.DeleteValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}
