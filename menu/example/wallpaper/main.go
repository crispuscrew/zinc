// Command wallpaper is a thumbnail wallpaper chooser built on the menu module's grid layout.
// It scans a directory for images, shows them as a filterable grid of thumbnails, and on Enter
// sets the chosen one as the wallpaper by running $WALLPAPER_CMD (with the image path appended)
// - or, if that is unset, just prints the path so the chooser stays compositor-agnostic and
// composable. It exists to prove the grid layout is reusable by an ordinary program: it depends
// on nothing but the menu package and the standard library, and builds static and cgo-free.
//
// Usage:
//
//	wallpaper [DIR]                      # DIR, else $WALLPAPER_DIR, else ~/Pictures/Wallpapers
//	WALLPAPER_CMD='swww img' wallpaper   # set via swww
//	WALLPAPER_CMD='swaybg -i' wallpaper  # set via swaybg
//	wallpaper | while read p; do ... done  # unset: prints the path, wire it yourself
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/crispuscrew/zinc/menu"
)

// imageExts are the raster formats the menu's thumbnailer can decode (pure-Go PNG/JPEG/GIF/WebP);
// other files in the directory are skipped so they do not show as blank tiles.
var imageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
}

func main() {
	dir := wallpaperDir()
	items, err := loadWallpapers(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(items) == 0 {
		fmt.Fprintf(os.Stderr, "no images found in %s\n", dir)
		os.Exit(1)
	}

	// On Enter: set the wallpaper (or print the path). Returning an error keeps the overlay up
	// and shows it in a banner, so a failing setter is reported in place rather than silently.
	activate := func(item menu.Item) error {
		return setWallpaper(item.Preview)
	}

	_, err = menu.Run(items, activate, menu.Options{
		Prompt: "wallpaper> ",
		Footer: "arrows move   enter set   type to filter   esc quit",
		AppID:  "zinc.wallpaper",
		Width:  920,
		Height: 640,
		Grid:   true,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// wallpaperDir picks the directory to scan: the first CLI argument, else $WALLPAPER_DIR, else
// ~/Pictures/Wallpapers (falling back to ~/Pictures when that does not exist).
func wallpaperDir() string {
	if len(os.Args) > 1 && os.Args[1] != "" {
		return os.Args[1]
	}
	if env := os.Getenv("WALLPAPER_DIR"); env != "" {
		return env
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	wallpapers := filepath.Join(home, "Pictures", "Wallpapers")
	if info, err := os.Stat(wallpapers); err == nil && info.IsDir() {
		return wallpapers
	}
	return filepath.Join(home, "Pictures")
}

// loadWallpapers returns one menu.Item per image file directly in dir (not recursive), sorted
// by name, with the filename as the label and the full path as the thumbnail Preview.
func loadWallpapers(dir string) ([]menu.Item, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read wallpaper directory %s: %w", dir, err)
	}
	var items []menu.Item
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !imageExts[strings.ToLower(filepath.Ext(name))] {
			continue
		}
		label := strings.TrimSuffix(name, filepath.Ext(name))
		items = append(items, menu.Item{Label: label, Preview: filepath.Join(dir, name)})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Label < items[j].Label })
	return items, nil
}

// setWallpaper applies the chosen image. With $WALLPAPER_CMD set (e.g. "swww img", "swaybg -i",
// "hyprctl hyprpaper preload") it runs that command with the path appended and reports any
// failure. With it unset it just prints the path to stdout, so the chooser works on any
// compositor and can be piped into whatever sets the wallpaper.
func setWallpaper(path string) error {
	command := strings.Fields(os.Getenv("WALLPAPER_CMD"))
	if len(command) == 0 {
		fmt.Println(path)
		return nil
	}
	args := append(command[1:], path)
	run := exec.Command(command[0], args...)
	run.Stdout = os.Stdout
	run.Stderr = os.Stderr
	if err := run.Run(); err != nil {
		return fmt.Errorf("%s: %w", command[0], err)
	}
	return nil
}
