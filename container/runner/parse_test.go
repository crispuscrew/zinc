package main

import "testing"

func TestParseVolumeSpec(t *testing.T) {
	for _, test := range []struct {
		spec                 string
		writable, executable bool
	}{
		{"/h:/c", false, false}, {"/h:/c:ro", false, false}, {"/h:/c:rw", true, false},
		{"/h:/c:exec", false, true}, {"/h:/c:noexec", false, false}, {"/h:/c:rw,exec", true, true}, {"/h:/c:rw,noexec", true, false},
	} {
		volume, err := parseVolumeSpec(test.spec)
		if err != nil || !volume.HostMounted || volume.HostMount != "/h" || volume.InnerMount != "/c" || volume.Writable != test.writable || volume.Executable != test.executable {
			t.Fatalf("%s: %+v %v", test.spec, volume, err)
		}
	}
	for _, spec := range []string{"/onlyhost", ":/c", "/h:", "/h:/c:rw:extra", "/h:/c:bogus", ""} {
		if _, err := parseVolumeSpec(spec); err == nil {
			t.Errorf("accepted %q", spec)
		}
	}
}

func TestParseRunArgs(t *testing.T) {
	name, execute, volumes, err := parseRunArgs([]string{"firefox"})
	if err != nil || name != "firefox" || execute || len(volumes) != 0 {
		t.Fatalf("%s %v %v %v", name, execute, volumes, err)
	}
	name, execute, volumes, err = parseRunArgs([]string{"firefox", "--exec", "-v", "/a:/a", "--volume", "/b:/b:rw"})
	if err != nil || name != "firefox" || !execute || len(volumes) != 2 || volumes[0].HostMount != "/a" || !volumes[1].Writable {
		t.Fatalf("%s %v %v %v", name, execute, volumes, err)
	}
	for _, flag := range []string{"-v=/c:/c", "--volume=/c:/c"} {
		name, _, volumes, err = parseRunArgs([]string{flag, "firefox"})
		if err != nil || name != "firefox" || len(volumes) != 1 || volumes[0].HostMount != "/c" {
			t.Fatalf("%s %v %v", name, volumes, err)
		}
	}
	for _, args := range [][]string{{"firefox", "-v"}, {"firefox", "--nope"}, {"-v", "/a:/a"}, {"firefox", "bar"}, {"firefox", "-v", "/onlyhost"}, {"firefox@work", "--instance=home"}} {
		if _, _, _, err := parseRunArgs(args); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
	name, _, _, err = parseRunArgs([]string{"--instance", "work", "firefox"})
	if err != nil || name != "firefox@work" {
		t.Fatalf("%s %v", name, err)
	}
}

func TestTerminalFlagGrammar(t *testing.T) {
	for _, args := range [][]string{{"demo", "--shell"}, {"--shell", "demo"}} {
		name, shell, err := parseTermArgs(args)
		if name != "demo" || !shell || err != nil {
			t.Fatalf("%s %v %v", name, shell, err)
		}
	}
	for _, args := range [][]string{nil, {"--bad", "demo"}, {"demo", "extra"}} {
		if _, _, err := parseTermArgs(args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
