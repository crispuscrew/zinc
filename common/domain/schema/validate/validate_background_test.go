package validate

import "testing"

func TestVMSerialTerminalMayRemainInBackground(t *testing.T) {
	cfg := baseVM()
	cfg.StartConditions.Terminal = true
	cfg.StopConditions.Background = true
	if err := Validate(cfg); err != nil {
		t.Fatalf("serial window lifetime does not require a guest attached command: %v", err)
	}
	cfg = baseCfg()
	cfg.StartConditions.Terminal = true
	cfg.StopConditions.Background = true
	requireError(t, cfg, "without Attached")
}
