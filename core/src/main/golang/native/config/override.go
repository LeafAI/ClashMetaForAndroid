package config

import (
	"io"
	"os"
	"strings"

	"github.com/metacubex/mihomo/constant"
)

type OverrideSlot int

const (
	OverrideSlotPersist OverrideSlot = iota
	OverrideSlotSession
)

const defaultPersistOverride = `{}`
const defaultSessionOverride = `{}`

var sessionOverride = defaultSessionOverride

func overridePersistPath() string {
	return constant.Path.Resolve("override.json")
}

func ReadOverride(slot OverrideSlot) string {
	switch slot {
	case OverrideSlotPersist:
		file, err := os.OpenFile(overridePersistPath(), os.O_RDONLY, 0600)
		if err != nil {
			return defaultPersistOverride
		}

		buf, err := io.ReadAll(file)
		if err != nil {
			return defaultPersistOverride
		}

		return string(buf)
	case OverrideSlotSession:
		return sessionOverride
	}

	return ""
}

func WriteOverride(slot OverrideSlot, content string) {
	switch slot {
	case OverrideSlotPersist:
		file, err := os.OpenFile(overridePersistPath(), os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0600)
		if err != nil {
			return
		}

		_, err = file.Write([]byte(content))
	case OverrideSlotSession:
		sessionOverride = content
	}
}

func rulesSourcePath() string {
	return constant.Path.Resolve("rules.yaml")
}

// ReadRuleSource returns the user's global rule source, or "" when none is set.
func ReadRuleSource() string {
	buf, err := os.ReadFile(rulesSourcePath())
	if err != nil {
		return ""
	}

	return string(buf)
}

// WriteRuleSource stores the user's global rule source. An empty source removes it.
func WriteRuleSource(content string) error {
	if strings.TrimSpace(content) == "" {
		if err := os.Remove(rulesSourcePath()); err != nil && !os.IsNotExist(err) {
			return err
		}

		return nil
	}

	return os.WriteFile(rulesSourcePath(), []byte(content), 0600)
}

func ClearOverride(slot OverrideSlot) {
	switch slot {
	case OverrideSlotPersist:
		_ = os.Remove(overridePersistPath())
	case OverrideSlotSession:
		sessionOverride = defaultSessionOverride
	}
}
