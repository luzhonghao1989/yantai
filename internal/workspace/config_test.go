package workspace

import (
	"strings"
	"testing"
)

func TestLargeContextConfiguration(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ContextLimit = 262144
	cfg.OutputLimit = 32768
	if err := ValidateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	saved, err := store.Config()
	if err != nil || saved.ContextLimit != 262144 || saved.OutputLimit != 32768 {
		t.Fatal("large context settings did not persist")
	}
	cfg.OutputLimit = 0
	if err := ValidateConfig(cfg); err == nil || !strings.Contains(err.Error(), "单次输出") {
		t.Fatal("output error is unclear")
	}
	cfg.ContextLimit = 4194304
	cfg.OutputLimit = 262144
	if err := ValidateConfig(cfg); err != nil {
		t.Fatal("future model limits rejected: ", err)
	}
	cfg.OutputLimit = 32768
	cfg.ContextLimit = 32768
	if err := ValidateConfig(cfg); err == nil || !strings.Contains(err.Error(), "必须大于") {
		t.Fatal("equal context/output should be rejected")
	}
}
