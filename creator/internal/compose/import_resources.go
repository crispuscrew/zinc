package compose

import (
	"math"
	"strconv"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func resources(service Service, note func(string, ...any)) schema.ResourcesMeta {
	result := schema.ResourcesMeta{PIDsLimit: service.PidsLimit}
	if service.Deploy == nil || service.Deploy.Resources.Limits == nil {
		return result
	}
	limits := service.Deploy.Resources.Limits
	if limits.CPUs != "" {
		if cores, err := strconv.ParseFloat(limits.CPUs, 64); err == nil && cores > 0 && !math.IsInf(cores, 0) {
			result.MaxCPUCores = cores
		} else {
			note("CPU limit %q could not be read: NO cpu limit imported", limits.CPUs)
		}
	}
	if limits.Memory != "" {
		if memory, ok := memoryMiB(limits.Memory); ok {
			result.MaxRamMiB = memory
		} else {
			note("memory limit %q could not be read as whole MiB: NO memory limit imported", limits.Memory)
		}
	}
	return result
}

func memoryMiB(text string) (int64, bool) {
	value := strings.TrimSpace(strings.ToLower(text))
	multiplier := int64(0)
	switch {
	case strings.HasSuffix(value, "gb"), strings.HasSuffix(value, "g"):
		multiplier, value = 1024, strings.TrimSuffix(strings.TrimSuffix(value, "b"), "g")
	case strings.HasSuffix(value, "mb"), strings.HasSuffix(value, "m"):
		multiplier, value = 1, strings.TrimSuffix(strings.TrimSuffix(value, "b"), "m")
	case strings.HasSuffix(value, "kb"), strings.HasSuffix(value, "k"):
		return 0, false
	default:
		value = strings.TrimSuffix(value, "b")
	}
	number, err := strconv.ParseInt(value, 10, 64)
	if err != nil || number <= 0 {
		return 0, false
	}
	if multiplier == 0 {
		if number < 1024*1024 || number%(1024*1024) != 0 {
			return 0, false
		}
		return number / (1024 * 1024), true
	}
	if number > math.MaxInt64/multiplier {
		return 0, false
	}
	return number * multiplier, true
}
