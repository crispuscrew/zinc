// Package session defines the command and environment shared by plans and attached launches.
package session

import (
	"maps"
	"slices"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

// Command uses shell grammar for both entrypoint forms, including quoting and pipelines.
func Command(start schema.StartConditions) []string {
	command := start.AttachedEntrypoint
	if strings.TrimSpace(command) == "" {
		command = start.Entrypoint
	}
	if strings.TrimSpace(command) == "" {
		return []string{"/bin/sh"}
	}
	return []string{"/bin/sh", "-c", command}
}

func Environment(start schema.StartConditions) map[string]string {
	environment := make(map[string]string, len(start.EntrypointEnv)+len(start.AttachedEnv))
	maps.Copy(environment, start.EntrypointEnv)
	maps.Copy(environment, start.AttachedEnv)
	return environment
}

func EnvArgs(environment map[string]string) []string {
	var args []string
	for _, name := range slices.Sorted(maps.Keys(environment)) {
		args = append(args, "-e", name+"="+environment[name])
	}
	return args
}
