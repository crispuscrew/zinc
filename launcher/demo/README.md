# Zinc launcher demo apps

A small set of ready-made app definitions so you can try the launchers without authoring
any apps of your own. They are plain schema-v2 app files - the same kind `zcc` writes to
`~/.config/zinc/apps`.

Run the picker against them (your real `~/.config/zinc/apps` is left untouched):

```sh
make -C launcher/gui demo     # the GUI picker (zlg)
make -C launcher/tui demo     # the TUI picker (zlt)
```

Each target builds the binary, copies these files into a throwaway
`bin/demo-home/zinc/apps/`, and runs the launcher with `XDG_CONFIG_HOME` pointed there.
Type to filter (try `n` - it matches both `ncdu` and `neovim`), arrows or `ctrl+n` /
`ctrl+p` to move, enter to launch, esc to quit.

Each app is a digest-pinned Alpine base plus an `apk add`, so launching one (you need `zcr`
on `$PATH`) actually builds and runs it: `firefox` is graphical, the rest open in a
terminal. To keep any of them, copy the file into `~/.config/zinc/apps/`.
