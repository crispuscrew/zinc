package audio

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	plan "github.com/crispuscrew/zinc/common/domain/audio"
)

// Prepare resolves every explicit target through the policy before returning a
// socket. Defaults are snapshotted by the policy. ALSA entries are returned for
// the caller's device mounts/QEMU ALSA backend; no ALSA device is opened here.
func Prepare(ctx context.Context, request Request) (*Session, error) {
	grants, err := plan.Build(request.Audio)
	if err != nil {
		return nil, err
	}
	session := newSession()
	session.ALSA = grants.ALSA
	if len(grants.PipeWire) == 0 {
		close(session.done)
		return session, nil
	}
	if !filepath.IsAbs(request.RuntimeDir) || request.AppID == "" || request.InstanceID == "" {
		return nil, fmt.Errorf("audio: absolute runtime directory, app and instance identity are required")
	}
	if strings.ContainsAny(request.AppID+request.InstanceID+request.RuntimeDir, "\x00\r\n") {
		return nil, errProtocol
	}
	if err := privateRuntime(request.RuntimeDir); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if request.Timeout == 0 {
		request.Timeout = 10 * time.Second
	}
	if request.Timeout < 0 {
		return nil, fmt.Errorf("audio: negative preparation timeout")
	}
	deadline := deadlineFor(ctx, request.Timeout)
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(entropy[:])
	instance := request.InstanceID + ":" + token
	fail := func(err error) (*Session, error) { session.finish(err); return nil, session.Err() }
	session.controller, err = connect(filepath.Join(request.RuntimeDir, "pipewire-0-manager"))
	if err != nil {
		return fail(err)
	}
	cancelRead := context.AfterFunc(ctx, func() { session.controller.socket.Close() })
	defer cancelRead()
	// The immutable server socket property authenticates this controller to policy.
	body, err := json.Marshal(struct {
		App        string           `json:"app"`
		Instance   string           `json:"instance"`
		Token      string           `json:"token"`
		Selections []plan.Selection `json:"selections"`
	}{request.AppID, instance, token, grants.PipeWire})
	if err != nil {
		return fail(err)
	}
	err = session.controller.hello("application.name", "Zinc audio broker",
		"zinc.audio.protocol", PolicyVersion, "zinc.audio.request", string(body))
	if err != nil {
		return fail(err)
	}
	props, err := awaitReady(session.controller, deadline)
	if err != nil {
		return fail(err)
	}
	if err := json.Unmarshal([]byte(props["zinc.audio.endpoints"]), &session.Endpoints); err != nil {
		return fail(err)
	}
	if err := checkEndpoints(session.Endpoints, token, grants.PipeWire); err != nil {
		return fail(err)
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	if err := session.createContext(request, instance, deadline); err != nil {
		return fail(err)
	}
	go session.supervise(ctx)
	return session, nil
}

func awaitReady(conn *connection, deadline time.Time) (map[string]string, error) {
	for {
		msg, err := conn.receive(deadline)
		if err != nil {
			return nil, fmt.Errorf("audio: Zinc WirePlumber policy missing or unready: %w", err)
		}
		if msg.object != 1 || msg.opcode != 0 {
			continue
		}
		props, err := clientProperties(msg.body)
		if err != nil {
			return nil, err
		}
		if err := policyError(props); err != nil {
			return nil, err
		}
		if props["zinc.audio.status"] == "ready" {
			if props["zinc.audio.version"] != PolicyVersion {
				return nil, fmt.Errorf("audio: incompatible policy version")
			}
			return props, nil
		}
	}
}

func policyError(props map[string]string) error {
	if status := props["zinc.audio.status"]; status == "error" || status == "revoked" {
		return fmt.Errorf("audio: policy %s: %s", status, props["zinc.audio.error"])
	}
	return nil
}
