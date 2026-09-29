package audioholder

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"strings"

	shared "github.com/crispuscrew/zinc/common/adapters/audio"
	plan "github.com/crispuscrew/zinc/common/domain/audio"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/ipc"
)

type Handle struct {
	*ipc.Child
	Ready Ready
}

func (handle *Handle) Check() error {
	select {
	case <-handle.Done:
		if err := handle.Err(); err != nil {
			return fmt.Errorf("audio holder failed: %w", err)
		}
		return fmt.Errorf("audio holder exited unexpectedly")
	default:
		return nil
	}
}

func Start(request shared.Request, diagnostic io.Writer) (*Handle, error) {
	child, err := ipc.Start([]string{Command, request.AppID, request.InstanceID}, request, diagnostic)
	if err != nil {
		return nil, err
	}
	var ready Ready
	err = ipc.Receive(child.Status, &ready)
	child.Status.Close()
	if err == nil && ready.Error != "" {
		err = fmt.Errorf("audio holder: %s", ready.Error)
	}
	if err == nil {
		err = validateReady(request, ready)
	}
	if err != nil {
		return nil, errors.Join(err, child.Close())
	}
	return &Handle{Child: child, Ready: ready}, nil
}

func validateReady(request shared.Request, ready Ready) error {
	grants, err := plan.Build(request.Audio)
	if err != nil {
		return err
	}
	socket := ready.Audio.Socket
	relative, err := filepath.Rel(request.RuntimeDir, socket)
	if err != nil || !filepath.IsAbs(socket) || strings.HasPrefix(relative, "..") ||
		!strings.HasPrefix(relative, "za-") || filepath.Base(socket) != "pipewire-0" || ready.Audio.Planning {
		return fmt.Errorf("audio holder returned an invalid private socket")
	}
	if !reflect.DeepEqual(ready.Environment, []string{"PIPEWIRE_REMOTE=" + socket}) || !reflect.DeepEqual(grants.ALSA, ready.Audio.ALSA) {
		return fmt.Errorf("audio holder returned unexpected environment or ALSA grants")
	}
	if len(ready.Audio.Endpoints) == 0 || len(ready.Audio.Endpoints) > len(grants.PipeWire) {
		return fmt.Errorf("invalid private endpoint count")
	}
	seen := map[string]bool{}
	for _, endpoint := range ready.Audio.Endpoints {
		if !strings.HasPrefix(endpoint.Name, "za.") || strings.ContainsAny(endpoint.Name, "\x00\r\n") || seen[endpoint.Name] {
			return fmt.Errorf("invalid private endpoint name")
		}
		found := false
		for _, grant := range grants.PipeWire {
			found = found || grant.Direction == endpoint.Direction
		}
		if !found {
			return fmt.Errorf("audio holder granted an unrequested direction")
		}
		seen[endpoint.Name] = true
	}
	for _, grant := range grants.PipeWire {
		found := false
		for _, endpoint := range ready.Audio.Endpoints {
			found = found || grant.Direction == endpoint.Direction
		}
		if !found {
			return fmt.Errorf("audio holder omitted a requested direction")
		}
	}
	return nil
}
