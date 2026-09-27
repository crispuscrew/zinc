package compose

import (
	"github.com/crispuscrew/zinc/common/domain/schema"
	"strings"
)

func importUser(value string, note func(string, ...any)) schema.InternalUserMeta {
	if value == "" {
		return schema.InternalUserMeta{}
	}
	name, _, group := strings.Cut(value, ":")
	if strings.TrimSpace(name) == "" || isNumeric(name) {
		note("user: %s was dropped: Zinc selects the user BY NAME, not numeric uid or group alone", value)
		return schema.InternalUserMeta{}
	}
	if group {
		note("user: %s became NonRootUserName %q; the group was dropped", value, name)
	}
	return schema.InternalUserMeta{UseNonRootUser: true, NonRootUserName: name}
}

func isNumeric(text string) bool {
	if text == "" {
		return false
	}
	for _, char := range text {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func appName(service string) string {
	var result strings.Builder
	for _, char := range strings.ToLower(strings.TrimSpace(service)) {
		switch {
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9', char == '.', char == '_', char == '-':
			result.WriteRune(char)
		default:
			result.WriteRune('-')
		}
	}
	name := strings.TrimLeft(result.String(), "._-")
	if name == "" {
		return "imported"
	}
	return name
}

func entrypoint(service Service) (string, []string) {
	source := service.Entrypoint
	if len(source) == 0 {
		source = service.Command
	}
	if len(source) == 0 {
		return "", nil
	}
	argv := []string(source)
	if len(source) == 1 {
		argv = strings.Fields(source[0])
	}
	if len(argv) == 0 {
		return "", nil
	}
	return argv[0], argv
}
