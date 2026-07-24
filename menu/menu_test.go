package menu

import "testing"

// toApps maps public items to internal picker rows field-for-field, in order, and resolves an
// empty Icon spec to no icon.
func TestToApps_MapsFieldsAndOrder(t *testing.T) {
	items := []Item{
		{Label: "firefox", Description: "browser", Group: "Web", Marked: true},
		{Label: "htop", Description: "monitor", Group: "System"},
	}
	apps := toApps(items)
	if len(apps) != len(items) {
		t.Fatalf("toApps returned %d rows, want %d", len(apps), len(items))
	}
	for index, app := range apps {
		item := items[index]
		if app.Name != item.Label || app.Description != item.Description || app.Group != item.Group {
			t.Errorf("row %d = {%q,%q,%q}, want {%q,%q,%q}", index, app.Name, app.Description, app.Group, item.Label, item.Description, item.Group)
		}
		if app.Running != item.Marked {
			t.Errorf("row %d Running = %v, want Marked %v", index, app.Running, item.Marked)
		}
		if app.Icon != nil {
			t.Errorf("row %d Icon should be nil for an empty Icon spec", index)
		}
	}
}

func TestOptionDefaults(t *testing.T) {
	if orString("", "fallback") != "fallback" || orString("set", "fallback") != "set" {
		t.Error("orString did not apply the fallback correctly")
	}
	if orInt(0, 720) != 720 || orInt(-5, 720) != 720 || orInt(500, 720) != 500 {
		t.Error("orInt did not apply the fallback correctly")
	}
}
