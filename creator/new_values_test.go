package main

import (
	"net"
	"reflect"
	"testing"
)

func TestParseResolution(t *testing.T) {
	for _, item := range []struct {
		text          string
		width, height int
	}{
		{"", 0, 0}, {" 1280 x 800 ", 1280, 800}, {"3840X2160", 3840, 2160},
	} {
		width, height, err := parseResolution(item.text)
		if err != nil || width != item.width || height != item.height {
			t.Errorf("%q: %dx%d %v", item.text, width, height, err)
		}
	}
	for _, text := range []string{"1920", "1920x1080x60", "widextall", "1920xtall"} {
		if _, _, err := parseResolution(text); err == nil {
			t.Errorf("accepted %q", text)
		}
	}
}

func TestResolveMac(t *testing.T) {
	for _, text := range []string{"", "02:11:22:33:44:55"} {
		if actual, err := resolveMac(text); actual != text || err != nil {
			t.Fatalf("literal MAC changed: %q %v", actual, err)
		}
	}
	seen := map[string]bool{}
	for range 32 {
		text, err := resolveMac("RANDOM")
		if err != nil {
			t.Fatal(err)
		}
		address, err := net.ParseMAC(text)
		if err != nil || seen[text] || address[0]&3 != 2 {
			t.Fatalf("invalid/random MAC: %q %v", text, err)
		}
		seen[text] = true
	}
}

func TestRawArgvAndEnvKeepExactValues(t *testing.T) {
	var args []string
	for _, value := range []string{"--label", "literal $(touch /tmp/never) with spaces", ""} {
		if err := appendArg(&args)(value); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(args, []string{"--label", "literal $(touch /tmp/never) with spaces", ""}) {
		t.Fatal(args)
	}
	var env map[string]string
	if err := envFlag(&env)("TOKEN=a=b c"); err != nil {
		t.Fatal(err)
	}
	if env["TOKEN"] != "a=b c" {
		t.Fatal(env)
	}
	if err := envFlag(&env)("TOKEN=second"); err == nil {
		t.Fatal("duplicate env silently won")
	}
	if err := envFlag(&env)("BARE"); err == nil {
		t.Fatal("implicit host env accepted")
	}
}

func TestStructuredFlagIsStrict(t *testing.T) {
	var values []string
	if err := decodeFlag(&values)("[one, 'two words']"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(values, []string{"one", "two words"}) {
		t.Fatal(values)
	}
	if err := decodeFlag(&values)("[one]\n---\n[two]"); err == nil {
		t.Fatal("extra YAML document ignored")
	}
}
