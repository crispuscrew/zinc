package tui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/crispuscrew/zinc/creator/internal/keys"
	"os/exec"
)

func loadSchemes() tea.Cmd {
	return func() tea.Msg {
		store, err := keys.DefaultStore()
		if err != nil {
			return errMsg{err}
		}
		names, err := store.List()
		if err != nil {
			return errMsg{err}
		}
		return schemesMsg{names}
	}
}

func setScheme(name string) tea.Cmd {
	return func() tea.Msg {
		store, err := keys.DefaultStore()
		if err != nil {
			return schemeSetMsg{err: err}
		}
		if err := store.SetActive(name); err != nil {
			return schemeSetMsg{err: err}
		}
		active, err := store.Load()
		return schemeSetMsg{active, err}
	}
}

func editScheme(name string) tea.Cmd {
	return func() tea.Msg {
		store, err := keys.DefaultStore()
		if err != nil {
			return errMsg{err}
		}
		_, path, err := store.EnsureEditable(name)
		if err != nil {
			return errMsg{err}
		}
		return schemeEditMsg{path}
	}
}

func openSchemeEditor(path string) tea.Cmd {
	argv := editorArgv(path)
	return tea.ExecProcess(exec.Command(argv[0], argv[1:]...), func(err error) tea.Msg {
		if err != nil {
			return errMsg{fmt.Errorf("editor: %w", err)}
		}
		return loadSchemes()()
	})
}
