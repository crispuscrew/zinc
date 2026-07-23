// Package ui is zlg's Wayland front-end: it opens a window, software-renders the picker
// (via internal/render) into a shared-memory buffer, and feeds keyboard input (decoded by
// internal/keymap) into the picker model. It speaks the Wayland wire protocol directly
// through go-wayland - no cgo - so zlg stays a static binary.
//
// This is the one part of zlg that needs a live compositor, so it is kept deliberately
// thin: every decision it drives (filter, cursor, launch, render) lives in the pure,
// unit-tested packages it calls. The event loop itself is validated by running zlg on a
// real Wayland session.
package ui

import (
	"fmt"
	"os"
	"syscall"

	"github.com/crispuscrew/zinc/launcher/gui/internal/keymap"
	"github.com/crispuscrew/zinc/launcher/gui/internal/picker"
	"github.com/crispuscrew/zinc/launcher/gui/internal/render"

	"github.com/rajveermalviya/go-wayland/wayland/client"
	xdg "github.com/rajveermalviya/go-wayland/wayland/stable/xdg-shell"
)

// Runner is the slice of the zcr delegate the UI drives: launch the chosen app and read
// which apps are currently running (for the indicator). Keeping it an interface lets the
// composition root inject the real zcr delegate.
type Runner interface {
	Launch(name string) error
	Running() (map[string]bool, error)
}

const (
	defaultWidth  = 680
	defaultHeight = 420
	pageStep      = 10
	edgeStep      = 1 << 20 // Home/End: jump far enough to clamp to the first/last row
)

// debugOn traces the Wayland handshake to stderr when ZLG_DEBUG is set, for diagnosing a
// compositor that rejects the connection.
var debugOn = os.Getenv("ZLG_DEBUG") != ""

func trace(format string, args ...any) {
	if debugOn {
		fmt.Fprintf(os.Stderr, "zlg: "+format+"\n", args...)
	}
}

// Run opens the picker window over apps, launching the selection through runner. It returns
// the launched app's name (empty if the user cancelled).
func Run(apps []picker.App, runner Runner) (string, error) {
	application := &app{
		model:  picker.New(apps),
		runner: runner,
		width:  defaultWidth,
		height: defaultHeight,
	}
	if running, err := runner.Running(); err == nil {
		application.model.SetRunning(running)
	}
	if err := application.connect(); err != nil {
		return "", err
	}
	defer application.cleanup()
	if err := application.loop(); err != nil {
		return "", err
	}
	return application.launched, nil
}

// app holds the live Wayland objects and the picker state for one run.
type app struct {
	model  *picker.Model
	runner Runner

	display    *client.Display
	ctx        *client.Context
	registry   *client.Registry
	compositor *client.Compositor
	shm        *client.Shm
	seat       *client.Seat
	keyboard   *client.Keyboard
	wmBase     *xdg.WmBase
	surface    *client.Surface
	xdgSurface *xdg.Surface
	toplevel   *xdg.Toplevel

	current *shmBuffer   // the buffer currently attached to the surface
	retired []*shmBuffer // resized-away buffers, kept alive until the compositor releases them

	width, height               int
	pendingWidth, pendingHeight int

	shift, ctrl bool

	launched string
	runErr   error
	closed   bool
}

// connect opens the display, binds the globals, and creates the window.
func (application *app) connect() error {
	display, err := client.Connect("")
	if err != nil {
		return fmt.Errorf("connect to the Wayland compositor (is a Wayland session running?): %w", err)
	}
	application.display = display
	application.ctx = display.Context()
	// Surface a compositor-reported protocol error with its message, rather than only the
	// follow-on socket EOF.
	display.SetErrorHandler(func(event client.DisplayErrorEvent) {
		application.runErr = fmt.Errorf("wayland protocol error (code %d): %s", event.Code, event.Message)
		application.closed = true
	})

	registry, err := display.GetRegistry()
	if err != nil {
		return err
	}
	application.registry = registry
	registry.SetGlobalHandler(application.handleGlobal)
	trace("connected; getting globals")

	// First roundtrip binds the advertised globals; the second lets the seat report its
	// capabilities (so the keyboard is set up before the window appears).
	if err := application.roundtrip(); err != nil {
		return application.fail(err)
	}
	if application.compositor == nil || application.shm == nil || application.wmBase == nil {
		return fmt.Errorf("the compositor is missing a required Wayland global "+
			"(have wl_compositor=%t wl_shm=%t xdg_wm_base=%t); run again with ZLG_DEBUG=1 to list what it advertised",
			application.compositor != nil, application.shm != nil, application.wmBase != nil)
	}
	if err := application.roundtrip(); err != nil {
		return application.fail(err)
	}
	trace("globals bound; creating the window")

	surface, err := application.compositor.CreateSurface()
	if err != nil {
		return application.fail(err)
	}
	application.surface = surface

	xdgSurface, err := application.wmBase.GetXdgSurface(surface)
	if err != nil {
		return application.fail(err)
	}
	application.xdgSurface = xdgSurface
	xdgSurface.SetConfigureHandler(application.handleSurfaceConfigure)

	toplevel, err := xdgSurface.GetToplevel()
	if err != nil {
		return application.fail(err)
	}
	application.toplevel = toplevel
	// set_title / set_app_id via sendStringRequest, not toplevel.SetTitle/SetAppId, whose
	// go-wayland encoders mis-declare the string length (see sendStringRequest). app_id is
	// what tiling compositors (niri, hyprland) match window rules against, so keep it set.
	if err := sendStringRequest(toplevel, toplevelSetTitleOpcode, "Zinc launcher"); err != nil {
		return application.fail(err)
	}
	if err := sendStringRequest(toplevel, toplevelSetAppIdOpcode, "zinc.launcher"); err != nil {
		return application.fail(err)
	}
	toplevel.SetConfigureHandler(application.handleToplevelConfigure)
	toplevel.SetCloseHandler(func(xdg.ToplevelCloseEvent) { application.closed = true })

	// The first surface commit (with no buffer) asks the compositor for the initial
	// configure; the buffer is attached in the configure handler.
	if err := surface.Commit(); err != nil {
		return application.fail(err)
	}
	trace("window committed; draining the first configure")
	// Read one round of events, so a protocol error in the setup above surfaces with its
	// message (via the error handler) rather than as a later broken pipe on the next write.
	if err := application.roundtrip(); err != nil {
		return application.fail(err)
	}
	if application.runErr != nil {
		return application.runErr
	}
	return nil
}

// fail surfaces a compositor protocol error if one is pending. A broken pipe on a write
// usually means the compositor already sent wl_display.error and closed, so read one more
// message to capture that message and prefer it over the raw socket error.
func (application *app) fail(cause error) error {
	if application.runErr == nil {
		_ = application.ctx.Dispatch() // may read a buffered wl_display.error
	}
	if application.runErr != nil {
		return application.runErr
	}
	return cause
}

// roundtrip dispatches events until a sync callback fires, i.e. until the compositor has
// processed everything sent so far.
func (application *app) roundtrip() error {
	callback, err := application.display.Sync()
	if err != nil {
		return err
	}
	done := false
	callback.SetDoneHandler(func(client.CallbackDoneEvent) { done = true })
	for !done {
		if err := application.ctx.Dispatch(); err != nil {
			return fmt.Errorf("wayland roundtrip: %w", err)
		}
	}
	return nil
}

// loop runs the event loop until the window is closed (by launch, cancel, or the
// compositor), returning any error that ended it.
func (application *app) loop() error {
	for !application.closed {
		if err := application.ctx.Dispatch(); err != nil {
			return fmt.Errorf("wayland dispatch: %w", err)
		}
	}
	return application.runErr
}

// bindGlobal binds a registry global with a correctly length-prefixed interface string.
// go-wayland v0.0.0-20230130's Registry.Bind writes the interface string's length field as
// the PADDED length, which leaves NUL padding inside the declared string; modern libwayland
// rejects that ("string has embedded nul") and closes the connection. This encodes the same
// wl_registry.bind request but with the true string length (len+1), padding the buffer
// separately, using go-wayland's exported wire primitives.
func bindGlobal(registry *client.Registry, name uint32, iface string, version uint32, proxy client.Proxy) error {
	const opcode = 0
	padded := client.PaddedLen(len(iface) + 1)
	total := 8 + 4 + (4 + padded) + 4 + 4
	buf := make([]byte, total)
	pos := 0
	client.PutUint32(buf[pos:pos+4], registry.ID())
	pos += 4
	client.PutUint32(buf[pos:pos+4], uint32(total<<16|opcode&0x0000ffff))
	pos += 4
	client.PutUint32(buf[pos:pos+4], name)
	pos += 4
	client.PutString(buf[pos:pos+(4+padded)], iface, len(iface)+1) // true length, not padded
	pos += 4 + padded
	client.PutUint32(buf[pos:pos+4], version)
	pos += 4
	client.PutUint32(buf[pos:pos+4], proxy.ID())
	return registry.Context().WriteMsg(buf, nil)
}

// sendStringRequest sends a single-string request (like xdg_toplevel.set_title) with a
// correctly length-prefixed string, working around the same go-wayland PutString bug that
// bindGlobal does: the generated encoders declare the string's length as its PADDED length,
// leaving NUL padding inside the declared bytes, which modern libwayland rejects as an
// "embedded nul". This encodes the request by hand with the true length (len+1).
func sendStringRequest(proxy client.Proxy, opcode uint32, value string) error {
	padded := client.PaddedLen(len(value) + 1)
	total := 8 + (4 + padded)
	buf := make([]byte, total)
	pos := 0
	client.PutUint32(buf[pos:pos+4], proxy.ID())
	pos += 4
	client.PutUint32(buf[pos:pos+4], uint32(total)<<16|opcode&0x0000ffff)
	pos += 4
	client.PutString(buf[pos:pos+(4+padded)], value, len(value)+1) // true length, not padded
	return proxy.Context().WriteMsg(buf, nil)
}

// xdg_toplevel request opcodes (from the xdg-shell protocol), used by sendStringRequest to
// bypass go-wayland's buggy string encoders.
const (
	toplevelSetTitleOpcode = 2
	toplevelSetAppIdOpcode = 3
)

// handleGlobal binds the globals zlg needs, at the version the compositor advertises (only
// v1 requests are used, so any advertised version is safe).
func (application *app) handleGlobal(event client.RegistryGlobalEvent) {
	trace("global advertised: %s v%d (name %d)", event.Interface, event.Version, event.Name)
	switch event.Interface {
	case "wl_compositor":
		compositor := client.NewCompositor(application.ctx)
		if bindGlobal(application.registry, event.Name, event.Interface, event.Version, compositor) == nil {
			application.compositor = compositor
			trace("bound wl_compositor v%d", event.Version)
		}
	case "wl_shm":
		shm := client.NewShm(application.ctx)
		if bindGlobal(application.registry, event.Name, event.Interface, event.Version, shm) == nil {
			application.shm = shm
			trace("bound wl_shm v%d", event.Version)
		}
	case "wl_seat":
		seat := client.NewSeat(application.ctx)
		if bindGlobal(application.registry, event.Name, event.Interface, event.Version, seat) == nil {
			application.seat = seat
			seat.SetCapabilitiesHandler(application.handleSeatCapabilities)
			trace("bound wl_seat v%d", event.Version)
		}
	case "xdg_wm_base":
		wmBase := xdg.NewWmBase(application.ctx)
		if bindGlobal(application.registry, event.Name, event.Interface, event.Version, wmBase) == nil {
			application.wmBase = wmBase
			wmBase.SetPingHandler(func(ping xdg.WmBasePingEvent) { wmBase.Pong(ping.Serial) })
			trace("bound xdg_wm_base v%d", event.Version)
		}
	}
}

// handleSeatCapabilities wires up the keyboard once the seat reports it has one.
func (application *app) handleSeatCapabilities(event client.SeatCapabilitiesEvent) {
	hasKeyboard := event.Capabilities&uint32(client.SeatCapabilityKeyboard) != 0
	if !hasKeyboard || application.keyboard != nil {
		return
	}
	keyboard, err := application.seat.GetKeyboard()
	if err != nil {
		return
	}
	application.keyboard = keyboard
	keyboard.SetKeyHandler(application.handleKey)
	keyboard.SetModifiersHandler(application.handleModifiers)
	// We use a fixed US layout (internal/keymap), so the compositor's keymap is unused;
	// close its fd to avoid a leak.
	keyboard.SetKeymapHandler(func(event client.KeyboardKeymapEvent) {
		if event.Fd >= 0 {
			syscall.Close(event.Fd)
		}
	})
}

// handleModifiers tracks Shift and Control from the standard xkb modifier indices (Shift is
// index 0, Control index 2 in the default keymap).
func (application *app) handleModifiers(event client.KeyboardModifiersEvent) {
	application.shift = event.ModsDepressed&(1<<0) != 0
	application.ctrl = event.ModsDepressed&(1<<2) != 0
}

// handleKey maps a keypress to a picker action and redraws.
func (application *app) handleKey(event client.KeyboardKeyEvent) {
	if event.State != uint32(client.KeyboardKeyStatePressed) {
		return
	}
	key := keymap.Decode(event.Key, application.shift)

	if application.ctrl {
		switch key.Rune {
		case 'u':
			application.model.ClearQuery()
			application.redraw()
		case 'n':
			application.model.MoveCursor(1)
			application.redraw()
		case 'p':
			application.model.MoveCursor(-1)
			application.redraw()
		case 'c':
			application.closed = true
		}
		return
	}

	if key.Printable() {
		application.model.Type(string(key.Rune))
		application.redraw()
		return
	}

	switch key.Special {
	case keymap.Enter:
		application.launchSelected()
	case keymap.Escape:
		application.closed = true
	case keymap.Backspace:
		application.model.Backspace()
		application.redraw()
	case keymap.Up:
		application.model.MoveCursor(-1)
		application.redraw()
	case keymap.Down:
		application.model.MoveCursor(1)
		application.redraw()
	case keymap.PageUp:
		application.model.MoveCursor(-pageStep)
		application.redraw()
	case keymap.PageDown:
		application.model.MoveCursor(pageStep)
		application.redraw()
	case keymap.Home:
		application.model.MoveCursor(-edgeStep)
		application.redraw()
	case keymap.End:
		application.model.MoveCursor(edgeStep)
		application.redraw()
	}
}

// launchSelected launches the highlighted app through zcr and closes the window; a launch
// error is surfaced by ending the loop with that error.
func (application *app) launchSelected() {
	selected, ok := application.model.Selected()
	if !ok {
		return
	}
	if err := application.runner.Launch(selected.Name); err != nil {
		application.runErr = fmt.Errorf("launch %s: %w", selected.Name, err)
	} else {
		application.launched = selected.Name
	}
	application.closed = true
}

// handleToplevelConfigure stashes the size the compositor suggests; it is applied on the
// paired xdg_surface configure.
func (application *app) handleToplevelConfigure(event xdg.ToplevelConfigureEvent) {
	application.pendingWidth = int(event.Width)
	application.pendingHeight = int(event.Height)
}

// handleSurfaceConfigure acknowledges the configure, applies any new size, and draws.
func (application *app) handleSurfaceConfigure(event xdg.SurfaceConfigureEvent) {
	application.xdgSurface.AckConfigure(event.Serial)
	if application.pendingWidth > 0 {
		application.width = application.pendingWidth
	}
	if application.pendingHeight > 0 {
		application.height = application.pendingHeight
	}
	if err := application.ensureBuffer(); err != nil {
		application.runErr = err
		application.closed = true
		return
	}
	application.redraw()
}

// shmBuffer is one wl_buffer plus the shared memory backing it. The mapping and the wl_buffer
// must stay alive - and the wl_buffer stays registered with go-wayland - until the compositor
// sends its release. Destroying a buffer the compositor still holds means a later release
// event lands on an unregistered object, which go-wayland's dispatch treats as fatal
// ("unable find sender"). So a resize retires the old buffer rather than destroying it.
type shmBuffer struct {
	buffer        *client.Buffer
	file          *os.File
	mmap          []byte
	width, height int
}

// destroy frees the buffer, mapping, and shm file. Only call it once the compositor is done
// with the buffer (on release, or at cleanup when the connection is closing anyway).
func (buf *shmBuffer) destroy() {
	if buf.buffer != nil {
		buf.buffer.Destroy()
		buf.buffer = nil
	}
	if buf.mmap != nil {
		syscall.Munmap(buf.mmap)
		buf.mmap = nil
	}
	if buf.file != nil {
		buf.file.Close()
		buf.file = nil
	}
}

// ensureBuffer makes application.current a buffer of the wanted size, retiring the old one
// (if the size changed) instead of destroying it while the compositor may still hold it.
func (application *app) ensureBuffer() error {
	width, height := application.width, application.height
	if width <= 0 {
		width = defaultWidth
	}
	if height <= 0 {
		height = defaultHeight
	}
	if application.current != nil && application.current.width == width && application.current.height == height {
		return nil
	}
	if application.current != nil {
		application.retire(application.current)
		application.current = nil
	}
	buf, err := application.newBuffer(width, height)
	if err != nil {
		return err
	}
	application.current = buf
	return nil
}

// newBuffer allocates an unlinked shm file, maps it, and wraps it in a wl_buffer.
func (application *app) newBuffer(width, height int) (*shmBuffer, error) {
	stride := width * 4
	size := stride * height
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	file, err := os.CreateTemp(dir, "zlg-shm-*")
	if err != nil {
		return nil, fmt.Errorf("create shm file: %w", err)
	}
	// Unlink now; the open fd (and the mapping) keep the memory alive.
	os.Remove(file.Name())
	if err := file.Truncate(int64(size)); err != nil {
		file.Close()
		return nil, err
	}
	data, err := syscall.Mmap(int(file.Fd()), 0, size, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("mmap shm: %w", err)
	}
	pool, err := application.shm.CreatePool(int(file.Fd()), int32(size))
	if err != nil {
		syscall.Munmap(data)
		file.Close()
		return nil, err
	}
	buffer, err := pool.CreateBuffer(0, int32(width), int32(height), int32(stride), uint32(client.ShmFormatArgb8888))
	pool.Destroy()
	if err != nil {
		syscall.Munmap(data)
		file.Close()
		return nil, err
	}
	return &shmBuffer{buffer: buffer, file: file, mmap: data, width: width, height: height}, nil
}

// retire holds a resized-away buffer until the compositor releases it, then frees it. A
// buffer the compositor already released while it was current simply never fires this
// handler and is reclaimed at cleanup instead.
func (application *app) retire(buf *shmBuffer) {
	application.retired = append(application.retired, buf)
	buf.buffer.SetReleaseHandler(func(client.BufferReleaseEvent) {
		trace("compositor released a retired buffer (%dx%d)", buf.width, buf.height)
		application.freeRetired(buf)
	})
}

// freeRetired drops buf from the retired list and frees it.
func (application *app) freeRetired(buf *shmBuffer) {
	for index, candidate := range application.retired {
		if candidate == buf {
			application.retired = append(application.retired[:index], application.retired[index+1:]...)
			buf.destroy()
			return
		}
	}
}

// redraw renders the model and blits it into the shm buffer as ARGB8888 (byte order
// B,G,R,A little-endian), then commits the surface.
//
// It reuses one buffer and does not wait for wl_buffer.release, so a burst of redraws can
// briefly tear - a cosmetic torn read (both sides map the same live file), never a crash.
// Double-buffering is a future refinement; for a redraw-on-keystroke launcher this is fine.
func (application *app) redraw() {
	buf := application.current
	if buf == nil || buf.mmap == nil {
		return
	}
	frame := render.Frame(application.model, buf.width, buf.height)
	src := frame.Pix
	dst := buf.mmap
	count := len(dst)
	if len(src) < count {
		count = len(src)
	}
	for index := 0; index+3 < count; index += 4 {
		dst[index+0] = src[index+2] // B
		dst[index+1] = src[index+1] // G
		dst[index+2] = src[index+0] // R
		dst[index+3] = src[index+3] // A
	}
	application.surface.Attach(buf.buffer, 0, 0)
	// Damage (surface-local, wl_surface v1) rather than DamageBuffer (v4), so we never
	// depend on binding wl_compositor at v4+. We redraw the whole surface, so full-surface
	// damage is exactly right.
	application.surface.Damage(0, 0, int32(buf.width), int32(buf.height))
	application.surface.Commit()
}

// cleanup frees every buffer (current and any awaiting release) and closes the connection.
func (application *app) cleanup() {
	if application.current != nil {
		application.current.destroy()
		application.current = nil
	}
	for _, buf := range application.retired {
		buf.destroy()
	}
	application.retired = nil
	if application.ctx != nil {
		application.ctx.Close()
	}
}
