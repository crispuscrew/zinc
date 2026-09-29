package app

import (
	"os"
	"path/filepath"
)

// A launch is serialised per app, because the check that refuses a second launch of a running
// app cannot see the first one yet.
//
// The refusal reads what the runtime reports as running, and the app's container does not appear
// there until the end of a launch that creates a pod, loads an nft ruleset and waits for a D-Bus
// proxy to answer. A second launch a second later therefore passes the same check, prepares the
// same objects, fails on the first that already exists, and runs the fail-closed teardown - which
// removes the FIRST launch's pod, proxy and sockets. Two launches racing left nothing running at
// all; a double Enter in a launcher was enough.
//
// The lock is held for the whole launch, so the second one waits and then sees the first as
// running and refuses it properly. It is per app rather than global: two different apps starting
// together is ordinary, and only one app's objects collide with its own.
//
// flock rather than a pid file, for the reason the multiterminal waiter uses it: the kernel drops
// it when the holder dies, so a launch killed halfway cannot wedge every later one.

// launchLock is a held lock, released by close.
type launchLock struct{ file *os.File }

// lockLaunch blocks until this app's launch lock is free and takes it. A runtime directory that
// cannot be made is not a reason to refuse the launch: the lock closes a race, and failing the
// whole launch because a directory is missing would be a worse answer than the race.
func lockLaunch(app string) *launchLock {
	root, err := runRoot()
	if err != nil {
		return nil
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil
	}
	file, err := lockFile(filepath.Join(root, "launch-"+app+".lock"), false)
	if err != nil {
		return nil
	}
	return &launchLock{file: file}
}

func (lock *launchLock) close() {
	if lock == nil || lock.file == nil {
		return
	}
	lock.file.Close() // releases the flock
}
