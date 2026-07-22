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

	shmFile             *os.File
	mmap                []byte
	buffer              *client.Buffer
	stride              int
	bufWidth, bufHeight int

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

	registry, err := display.GetRegistry()
	if err != nil {
		return err
	}
	application.registry = registry
	registry.SetGlobalHandler(application.handleGlobal)

	// First roundtrip binds the advertised globals; the second lets the seat report its
	// capabilities (so the keyboard is set up before the window appears).
	if err := application.roundtrip(); err != nil {
		return err
	}
	if application.compositor == nil || application.shm == nil || application.wmBase == nil {
		return fmt.Errorf("the compositor is missing a required Wayland global (wl_compositor / wl_shm / xdg_wm_base)")
	}
	if err := application.roundtrip(); err != nil {
		return err
	}

	surface, err := application.compositor.CreateSurface()
	if err != nil {
		return err
	}
	application.surface = surface

	xdgSurface, err := application.wmBase.GetXdgSurface(surface)
	if err != nil {
		return err
	}
	application.xdgSurface = xdgSurface
	xdgSurface.SetConfigureHandler(application.handleSurfaceConfigure)

	toplevel, err := xdgSurface.GetToplevel()
	if err != nil {
		return err
	}
	application.toplevel = toplevel
	toplevel.SetTitle("Zinc launcher")
	toplevel.SetConfigureHandler(application.handleToplevelConfigure)
	toplevel.SetCloseHandler(func(xdg.ToplevelCloseEvent) { application.closed = true })

	// The first surface commit (with no buffer) asks the compositor for the initial
	// configure; the buffer is attached in the configure handler.
	return surface.Commit()
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

// handleGlobal binds the globals zlg needs, at a version it understands.
func (application *app) handleGlobal(event client.RegistryGlobalEvent) {
	switch event.Interface {
	case "wl_compositor":
		compositor := client.NewCompositor(application.ctx)
		if application.registry.Bind(event.Name, event.Interface, pickVersion(event.Version, 4), compositor) == nil {
			application.compositor = compositor
		}
	case "wl_shm":
		shm := client.NewShm(application.ctx)
		if application.registry.Bind(event.Name, event.Interface, pickVersion(event.Version, 1), shm) == nil {
			application.shm = shm
		}
	case "wl_seat":
		seat := client.NewSeat(application.ctx)
		if application.registry.Bind(event.Name, event.Interface, pickVersion(event.Version, 5), seat) == nil {
			application.seat = seat
			seat.SetCapabilitiesHandler(application.handleSeatCapabilities)
		}
	case "xdg_wm_base":
		wmBase := xdg.NewWmBase(application.ctx)
		if application.registry.Bind(event.Name, event.Interface, pickVersion(event.Version, 2), wmBase) == nil {
			application.wmBase = wmBase
			wmBase.SetPingHandler(func(ping xdg.WmBasePingEvent) { wmBase.Pong(ping.Serial) })
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

// ensureBuffer (re)creates the shared-memory buffer when the size changes or on first use.
func (application *app) ensureBuffer() error {
	width, height := application.width, application.height
	if width <= 0 {
		width = defaultWidth
	}
	if height <= 0 {
		height = defaultHeight
	}
	if application.buffer != nil && application.bufWidth == width && application.bufHeight == height {
		return nil
	}
	application.releaseBuffer()

	stride := width * 4
	size := stride * height
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	file, err := os.CreateTemp(dir, "zlg-shm-*")
	if err != nil {
		return fmt.Errorf("create shm file: %w", err)
	}
	// Unlink now; the open fd (and the mapping) keep the memory alive.
	os.Remove(file.Name())
	if err := file.Truncate(int64(size)); err != nil {
		file.Close()
		return err
	}
	data, err := syscall.Mmap(int(file.Fd()), 0, size, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		file.Close()
		return fmt.Errorf("mmap shm: %w", err)
	}
	pool, err := application.shm.CreatePool(int(file.Fd()), int32(size))
	if err != nil {
		syscall.Munmap(data)
		file.Close()
		return err
	}
	buffer, err := pool.CreateBuffer(0, int32(width), int32(height), int32(stride), uint32(client.ShmFormatArgb8888))
	pool.Destroy()
	if err != nil {
		syscall.Munmap(data)
		file.Close()
		return err
	}

	application.shmFile = file
	application.mmap = data
	application.buffer = buffer
	application.stride = stride
	application.bufWidth, application.bufHeight = width, height
	return nil
}

// releaseBuffer frees the current buffer, mapping, and shm file (if any).
func (application *app) releaseBuffer() {
	if application.buffer != nil {
		application.buffer.Destroy()
		application.buffer = nil
	}
	if application.mmap != nil {
		syscall.Munmap(application.mmap)
		application.mmap = nil
	}
	if application.shmFile != nil {
		application.shmFile.Close()
		application.shmFile = nil
	}
}

// redraw renders the model and blits it into the shm buffer as ARGB8888 (byte order
// B,G,R,A little-endian), then commits the surface.
func (application *app) redraw() {
	if application.buffer == nil || application.mmap == nil {
		return
	}
	frame := render.Frame(application.model, application.bufWidth, application.bufHeight)
	src := frame.Pix
	dst := application.mmap
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
	application.surface.Attach(application.buffer, 0, 0)
	application.surface.DamageBuffer(0, 0, int32(application.bufWidth), int32(application.bufHeight))
	application.surface.Commit()
}

// cleanup releases the buffer and closes the connection.
func (application *app) cleanup() {
	application.releaseBuffer()
	if application.ctx != nil {
		application.ctx.Close()
	}
}

// pickVersion returns the lower of the advertised and the maximum version zlg supports.
func pickVersion(advertised, supported uint32) uint32 {
	if advertised < supported {
		return advertised
	}
	return supported
}
