package disk

import (
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

// A batch file fails at the byte level, not the logic level: cmd.exe reads it a line at a
// time and misparses anything it does not like without saying so. These are the four ways
// this particular script would break on a guest nobody can log into to debug.
func TestWindowsSetup_ParsesAsABatchFile(t *testing.T) {
	script := windowsSetup()

	// cmd.exe wants CRLF. With bare newlines it mangles labels and block continuations.
	for index, line := range strings.Split(script, "\r\n") {
		if strings.Contains(line, "\n") {
			t.Fatalf("line %d has a bare newline: %q", index, line)
		}
	}
	if !strings.HasSuffix(script, "\r\n") {
		t.Error("the last line needs its terminator; cmd.exe drops an unterminated one")
	}

	// Every `call :label` has to land somewhere, or the script silently does nothing.
	for _, label := range []string{":stage", ":elevated"} {
		if !strings.Contains(script, "\r\n"+label+"\r\n") {
			t.Errorf("%s is jumped to but never defined", label)
		}
	}

	// A bare ) inside a parenthesised block closes it early, so any echo in one must escape
	// its closing paren. This is the line that reports a failure - the one path where being
	// wrong costs the most.
	for _, line := range strings.Split(script, "\r\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "echo ") {
			continue
		}
		text := strings.TrimPrefix(trimmed, "echo ")
		if strings.Contains(text, ")") && !strings.Contains(text, "^)") {
			t.Errorf("echo with an unescaped ): %q", trimmed)
		}
	}
}

// The script is for a guest with no virtio drivers. A virtio guest drives everything already,
// and its provisioning disc is cloud-init's - putting a Windows batch file on it would be a
// stray file on a volume another program owns.
func TestSeedFiles_TheScriptGoesOnlyToAGuestThatNeedsIt(t *testing.T) {
	compatible := vmCfg()
	compatible.VirtualizationMeta.Devices = schema.VMDevicesCompatible
	files, err := seedFiles(compatible)
	if err != nil {
		t.Fatalf("seedFiles: %v", err)
	}
	if _, ok := files["zinc-setup.cmd"]; !ok {
		t.Error("a compatible guest should get the setup script")
	}
	for _, name := range []string{"user-data", "meta-data"} {
		if _, ok := files[name]; !ok {
			t.Errorf("%s should still be there: the disc is cloud-init's too", name)
		}
	}

	virtio := vmCfg()
	virtio.VirtualizationMeta.Devices = schema.VMDevicesVirtio
	files, err = seedFiles(virtio)
	if err != nil {
		t.Fatalf("seedFiles: %v", err)
	}
	if _, ok := files["zinc-setup.cmd"]; ok {
		t.Error("a virtio guest drives everything already and should not get the script")
	}
}

// The whole point of the script is that the guest cannot be told a drive letter in advance:
// it depends on how many discs are attached and in what order. It has to find the disc by
// looking for something only that disc has, and it has to cope with an older virtio-win
// build whose folders are named for Windows 10.
func TestWindowsSetup_FindsTheDiscItself(t *testing.T) {
	script := windowsSetup()

	if strings.Count(script, "for %%D in (D E F G H I J K L M N O P Q R S T U V W X Y Z) do") != 2 {
		t.Error("both the w11 and the w10 search should sweep every drive letter")
	}
	for _, probe := range []string{`%%D:\viogpudo\w11\amd64\viogpudo.inf`, `%%D:\viogpudo\w10\amd64\viogpudo.inf`} {
		if !strings.Contains(script, probe) {
			t.Errorf("the disc is not identified by %s", probe)
		}
	}
	if !strings.Contains(script, "if not defined DISC") {
		t.Error("a missing driver disc must be reported, not run into")
	}
}

// All three drivers, not just the display one. Switching an app to Devices: Virtio without
// viostor already staged leaves Windows unable to see its own boot disk, and the whole value
// of this script is that it is run once, before any of those switches.
func TestWindowsSetup_StagesEveryDriverTheMachineCanUse(t *testing.T) {
	script := windowsSetup()

	for _, inf := range []string{`viogpudo\%REL%\amd64\viogpudo.inf`, `viostor\%REL%\amd64\viostor.inf`, `NetKVM\%REL%\amd64\netkvm.inf`} {
		if !strings.Contains(script, inf) {
			t.Errorf("%s is never staged", inf)
		}
	}
	if !strings.Contains(script, "pnputil /add-driver %1 /install") {
		t.Error("drivers should go into the driver store, so hardware that appears later binds on its own")
	}
	// Elevation is not optional: pnputil without an administrator token fails on every
	// driver, and a double-clicked script has no token.
	if !strings.Contains(script, "net session >nul 2>&1") || !strings.Contains(script, "-Verb RunAs") {
		t.Error("the script must check for elevation and re-launch itself")
	}
}
