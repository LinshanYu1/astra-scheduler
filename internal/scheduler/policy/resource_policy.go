package policy

import (
	"strconv"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
)

func maxResources(profile *astrav1alpha1.AIWorkloadProfile) astrav1alpha1.ResourceSummary {
	if profile == nil || profile.Spec.Resources == nil {
		return astrav1alpha1.ResourceSummary{}
	}
	return maxResourcesFromRequest(profile.Spec.Resources)
}

func maxResourcesFromRequest(request *astrav1alpha1.WorkloadResourceRequest) astrav1alpha1.ResourceSummary {
	if request == nil {
		return astrav1alpha1.ResourceSummary{}
	}
	if request.Max != nil {
		return *request.Max
	}
	if request.Preferred != nil {
		return *request.Preferred
	}
	if request.Required != nil {
		return *request.Required
	}
	return astrav1alpha1.ResourceSummary{}
}

func requiredResourcesFromRequest(request *astrav1alpha1.WorkloadResourceRequest) astrav1alpha1.ResourceSummary {
	if request == nil || request.Required == nil {
		return astrav1alpha1.ResourceSummary{}
	}
	return *request.Required
}

func addResourceSummary(left, right astrav1alpha1.ResourceSummary) astrav1alpha1.ResourceSummary {
	return astrav1alpha1.ResourceSummary{
		GPUCount:               left.GPUCount + right.GPUCount,
		GPUMemoryGiB:           left.GPUMemoryGiB + right.GPUMemoryGiB,
		KVCacheGiB:             left.KVCacheGiB + right.KVCacheGiB,
		PrefillTokensPerSecond: left.PrefillTokensPerSecond + right.PrefillTokensPerSecond,
		DecodeTokensPerSecond:  left.DecodeTokensPerSecond + right.DecodeTokensPerSecond,
		TotalTokensPerSecond:   left.TotalTokensPerSecond + right.TotalTokensPerSecond,
	}
}

func resourceSummaryFits(required, capacity astrav1alpha1.ResourceSummary) (bool, string) {
	if required.GPUCount > capacity.GPUCount {
		return false, resourceFitReason("gpuCount", required.GPUCount, capacity.GPUCount)
	}
	if required.GPUMemoryGiB > capacity.GPUMemoryGiB {
		return false, resourceFitReason("gpuMemoryGiB", required.GPUMemoryGiB, capacity.GPUMemoryGiB)
	}
	if required.KVCacheGiB > capacity.KVCacheGiB {
		return false, resourceFitReason("kvCacheGiB", required.KVCacheGiB, capacity.KVCacheGiB)
	}
	if required.PrefillTokensPerSecond > capacity.PrefillTokensPerSecond {
		return false, resourceFitReason("prefillTokensPerSecond", required.PrefillTokensPerSecond, capacity.PrefillTokensPerSecond)
	}
	if required.DecodeTokensPerSecond > capacity.DecodeTokensPerSecond {
		return false, resourceFitReason("decodeTokensPerSecond", required.DecodeTokensPerSecond, capacity.DecodeTokensPerSecond)
	}
	if required.TotalTokensPerSecond > capacity.TotalTokensPerSecond {
		return false, resourceFitReason("totalTokensPerSecond", required.TotalTokensPerSecond, capacity.TotalTokensPerSecond)
	}
	return true, "resource envelope fits"
}

func resourceFitReason(resource string, required, capacity int32) string {
	return "insufficient " + resource + " envelope: required=" + int32ToString(required) + " capacity=" + int32ToString(capacity)
}

func int32ToString(value int32) string {
	return strconv.FormatInt(int64(value), 10)
}
