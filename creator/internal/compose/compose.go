// Package compose translates app definitions with explicit loss reports.
package compose

import "gopkg.in/yaml.v3"

type Project struct {
	Name     string             `yaml:"name,omitempty"`
	Services map[string]Service `yaml:"services"`
	Networks map[string]Network `yaml:"networks,omitempty"`
}

type Service struct {
	Image         string               `yaml:"image,omitempty"`
	ContainerName string               `yaml:"container_name,omitempty"`
	Entrypoint    StringList           `yaml:"entrypoint,omitempty"`
	Command       StringList           `yaml:"command,omitempty"`
	User          string               `yaml:"user,omitempty"`
	Restart       string               `yaml:"restart,omitempty"`
	CapDrop       []string             `yaml:"cap_drop,omitempty"`
	CapAdd        []string             `yaml:"cap_add,omitempty"`
	SecurityOpt   []string             `yaml:"security_opt,omitempty"`
	PidsLimit     int64                `yaml:"pids_limit,omitempty"`
	Ports         StringList           `yaml:"ports,omitempty"`
	Expose        StringList           `yaml:"expose,omitempty"`
	Volumes       StringList           `yaml:"volumes,omitempty"`
	Networks      StringList           `yaml:"networks,omitempty"`
	DependsOn     Dependencies         `yaml:"depends_on,omitempty"`
	Healthcheck   *Healthcheck         `yaml:"healthcheck,omitempty"`
	Deploy        *Deploy              `yaml:"deploy,omitempty"`
	Labels        Labels               `yaml:"labels,omitempty"`
	DNS           StringList           `yaml:"dns,omitempty"`
	Environment   Environment          `yaml:"environment,omitempty"`
	ReadOnly      bool                 `yaml:"read_only,omitempty"`
	NetworkMode   string               `yaml:"network_mode,omitempty"`
	Unrepresented map[string]yaml.Node `yaml:",inline"`
}

type Network struct {
	Internal bool `yaml:"internal,omitempty"`
}
type Depend struct {
	Condition string `yaml:"condition,omitempty"`
}

const (
	ConditionStarted = "service_started"
	ConditionHealthy = "service_healthy"
)

type Healthcheck struct {
	Test     StringList `yaml:"test,omitempty"`
	Interval string     `yaml:"interval,omitempty"`
	Timeout  string     `yaml:"timeout,omitempty"`
	Retries  int        `yaml:"retries,omitempty"`
}

type Deploy struct {
	Resources Resources `yaml:"resources,omitempty"`
}
type Resources struct {
	Limits *Limits `yaml:"limits,omitempty"`
}
type Limits struct {
	CPUs   string `yaml:"cpus,omitempty"`
	Memory string `yaml:"memory,omitempty"`
}
