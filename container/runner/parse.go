package main

import (
	"fmt"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

const runUsage = "usage: zcr run <app> [--exec] [-v HOST:CONTAINER[:OPTIONS]]..."

func parseRunArgs(argv []string) (name string, execute bool, volumes []schema.Volume, err error) {
	var instance string
	for index := 0; index < len(argv); index++ {
		arg := argv[index]
		flag, value, attached := strings.Cut(arg, "=")
		switch {
		case arg == "--exec":
			execute = true
		case flag == "-v" || flag == "--volume" || flag == "--instance":
			if !attached {
				index++
				if index >= len(argv) {
					return "", false, nil, fmt.Errorf("%s: missing value", flag)
				}
				value = argv[index]
			}
			if flag == "--instance" {
				instance = value
				continue
			}
			volume, failure := parseVolumeSpec(value)
			if failure != nil {
				return "", false, nil, failure
			}
			volumes = append(volumes, volume)
		case strings.HasPrefix(arg, "-"):
			return "", false, nil, fmt.Errorf("unknown flag %q\n%s", arg, runUsage)
		case name == "":
			name = arg
		default:
			return "", false, nil, fmt.Errorf("unexpected argument %q\n%s", arg, runUsage)
		}
	}
	if name == "" {
		return "", false, nil, fmt.Errorf("%s", runUsage)
	}
	if instance != "" {
		if strings.Contains(name, "@") {
			return "", false, nil, fmt.Errorf("%q already names an instance; give one or the other", name)
		}
		name += "@" + instance
	}
	return name, execute, volumes, nil
}

func parseVolumeSpec(spec string) (schema.Volume, error) {
	fields := strings.Split(spec, ":")
	if len(fields) < 2 || len(fields) > 3 {
		return schema.Volume{}, fmt.Errorf("--volume %q: want HOST:CONTAINER[:OPTIONS]", spec)
	}
	if strings.TrimSpace(fields[0]) == "" || strings.TrimSpace(fields[1]) == "" {
		return schema.Volume{}, fmt.Errorf("--volume %q: empty HOST or CONTAINER path", spec)
	}
	volume := schema.Volume{HostMounted: true, HostMount: fields[0], InnerMount: fields[1]}
	if len(fields) == 3 {
		for _, option := range strings.Split(fields[2], ",") {
			switch strings.TrimSpace(option) {
			case "rw":
				volume.Writable = true
			case "ro":
				volume.Writable = false
			case "exec":
				volume.Executable = true
			case "noexec":
				volume.Executable = false
			default:
				return schema.Volume{}, fmt.Errorf("--volume %q: unknown option %q (want rw, ro, exec, noexec)", spec, option)
			}
		}
	}
	return volume, nil
}

func parseTermArgs(argv []string) (name string, shell bool, err error) {
	for _, arg := range argv {
		switch {
		case arg == "--shell":
			shell = true
		case strings.HasPrefix(arg, "-"):
			return "", false, fmt.Errorf("unknown flag %q\nusage: zcr term <app> [--shell]", arg)
		case name == "":
			name = arg
		default:
			return "", false, fmt.Errorf("unexpected argument %q\nusage: zcr term <app> [--shell]", arg)
		}
	}
	if name == "" {
		return "", false, fmt.Errorf("usage: zcr term <app> [--shell]")
	}
	return name, shell, nil
}
