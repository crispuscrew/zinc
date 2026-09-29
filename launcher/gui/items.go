package main

import (
	"sort"

	"github.com/crispuscrew/zinc/launcher/common/runner"
	"github.com/crispuscrew/zinc/launcher/common/store"
	"github.com/crispuscrew/zinc/menu"
)

// loadItems keeps store keys as labels and action identities. Undecodable apps
// remain listed by key so attempting to launch them surfaces the config error.
func loadItems() ([]menu.Item, error) {
	appStore, err := store.Default()
	if err != nil {
		return nil, err
	}
	names, err := appStore.List()
	if err != nil {
		return nil, err
	}
	running, _ := runner.Running() // best-effort; either runtime may be unavailable
	items := make([]menu.Item, 0, len(names))
	for _, name := range names {
		item := menu.Item{Label: name, Marked: running[name]}
		if config, err := appStore.LoadResolved(name); err == nil {
			item.Description = config.LauncherMeta.Description
			item.Group = config.LauncherMeta.Group
			item.Icon = config.LauncherMeta.Icon
		}
		items = append(items, item)
	}
	// Group first, then key; ungrouped apps form the trailing "Other" section.
	sort.Slice(items, func(left, right int) bool {
		leftGroup, rightGroup := items[left].Group, items[right].Group
		if leftGroup != rightGroup {
			if leftGroup == "" {
				return false
			}
			if rightGroup == "" {
				return true
			}
			return leftGroup < rightGroup
		}
		return items[left].Label < items[right].Label
	})
	return items, nil
}
