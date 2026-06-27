package agent

import (
	"time"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const hourlyForecastBucketCount = 24

func buildHourlyForecast(
	allocation *astrav1alpha1.AIResourceAllocation,
	generatedAt *metav1.Time,
) *astrav1alpha1.HourlyResourceForecast {
	if allocation == nil {
		return emptyHourlyForecast(generatedAt)
	}

	required := requiredForecastResources(allocation)
	maximum := maxForecastResources(allocation)
	buckets := make([]astrav1alpha1.ResourceSummary, 0, hourlyForecastBucketCount)
	for hour := 0; hour < hourlyForecastBucketCount; hour++ {
		if hourInForecastWindows(allocation.Spec.TimeWindows, hour) && !resourceSummaryIsZero(maximum) {
			buckets = append(buckets, maximum)
			continue
		}
		buckets = append(buckets, required)
	}

	return &astrav1alpha1.HourlyResourceForecast{
		GeneratedAt:     generatedAt,
		Timezone:        forecastTimezone(allocation.Spec.TimeWindows),
		ResourceBuckets: buckets,
	}
}

func emptyHourlyForecast(generatedAt *metav1.Time) *astrav1alpha1.HourlyResourceForecast {
	return &astrav1alpha1.HourlyResourceForecast{
		GeneratedAt:     generatedAt,
		ResourceBuckets: make([]astrav1alpha1.ResourceSummary, hourlyForecastBucketCount),
	}
}

func requiredForecastResources(allocation *astrav1alpha1.AIResourceAllocation) astrav1alpha1.ResourceSummary {
	if allocation.Spec.ResourceRequest != nil && allocation.Spec.ResourceRequest.Required != nil {
		return *allocation.Spec.ResourceRequest.Required
	}
	if allocation.Spec.Resources != nil {
		return *allocation.Spec.Resources
	}
	return astrav1alpha1.ResourceSummary{}
}

func maxForecastResources(allocation *astrav1alpha1.AIResourceAllocation) astrav1alpha1.ResourceSummary {
	if allocation.Spec.ResourceRequest != nil {
		if allocation.Spec.ResourceRequest.Max != nil {
			return *allocation.Spec.ResourceRequest.Max
		}
		if allocation.Spec.ResourceRequest.Preferred != nil {
			return *allocation.Spec.ResourceRequest.Preferred
		}
	}
	return requiredForecastResources(allocation)
}

func forecastTimezone(windows []astrav1alpha1.TimeWindow) string {
	for _, window := range windows {
		if window.Timezone != "" {
			return window.Timezone
		}
	}
	return ""
}

func hourInForecastWindows(windows []astrav1alpha1.TimeWindow, hour int) bool {
	if hour < 0 || hour > 23 {
		return false
	}

	minute := hour * 60
	for _, window := range windows {
		start, ok := parseForecastMinute(window.Start)
		if !ok {
			continue
		}
		end, ok := parseForecastMinute(window.End)
		if !ok {
			continue
		}
		if minuteInForecastWindow(minute, start, end) {
			return true
		}
	}
	return false
}

func parseForecastMinute(value string) (int, bool) {
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return 0, false
	}
	return parsed.Hour()*60 + parsed.Minute(), true
}

func minuteInForecastWindow(current, start, end int) bool {
	if start == end {
		return true
	}
	if start < end {
		return current >= start && current < end
	}
	return current >= start || current < end
}

func resourceSummaryIsZero(value astrav1alpha1.ResourceSummary) bool {
	return value.GPUCount == 0 &&
		value.GPUMemoryGiB == 0 &&
		value.KVCacheGiB == 0 &&
		value.PrefillTokensPerSecond == 0 &&
		value.DecodeTokensPerSecond == 0 &&
		value.TotalTokensPerSecond == 0
}
