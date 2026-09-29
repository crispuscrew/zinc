package tui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/creator/internal/backend"
)

func writeDraft(service backend.Service, cfg schema.AppConfig) tea.Cmd {
	return func() tea.Msg {
		data, err := service.Marshal(cfg)
		if err != nil {
			return errMsg{err}
		}
		file, err := os.CreateTemp("", "zc-*.yaml")
		if err != nil {
			return errMsg{err}
		}
		_, writeErr := file.Write(data)
		if err := errors.Join(writeErr, file.Close()); err != nil {
			return errMsg{errors.Join(err, os.Remove(file.Name()))}
		}
		return editReadyMsg{file.Name()}
	}
}

func openEditor(service backend.Service, path string) tea.Cmd {
	argv := editorArgv(path)
	return tea.ExecProcess(exec.Command(argv[0], argv[1:]...), func(err error) tea.Msg {
		defer os.Remove(path)
		if err != nil {
			return editedMsg{err: fmt.Errorf("editor: %w", err)}
		}
		cfg, err := service.LoadFile(path)
		return editedMsg{cfg, err}
	})
}

func editorArgv(path string) []string {
	argv := strings.Fields(os.Getenv("EDITOR"))
	if len(argv) == 0 {
		argv = []string{"vim"}
	}
	return append(argv, path)
}
