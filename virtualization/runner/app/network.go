package app

import (
	"fmt"

	"github.com/crispuscrew/zinc/common/domain/nftrules"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/netns"
)

func (svc Service) NetCounters(name string) ([]nftrules.RuleCounter, bool, error) {
	state, err := svc.State(name)
	if err != nil {
		return nil, false, err
	}
	if !state.Alive {
		return nil, false, fmt.Errorf("%s is not running; counters belong to a running guest's namespace", name)
	}
	namespaced, err := netns.Namespaced(state.PID)
	if err != nil || !namespaced {
		return nil, false, err
	}
	raw, err := netns.Counters(state.PID)
	if err != nil {
		return nil, true, err
	}
	counters, err := nftrules.ParseCounters(raw)
	return counters, true, err
}
