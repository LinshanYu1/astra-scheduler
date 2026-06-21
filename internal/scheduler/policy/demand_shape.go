package policy

import (
	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
)

type DemandShapeResourceWeights map[astrav1alpha1.ResourceName]float64

type DemandShapeResourceMapping map[astrav1alpha1.DemandShape]DemandShapeResourceWeights

func DefaultDemandShapeResourceMapping() DemandShapeResourceMapping {
	return DemandShapeResourceMapping{
		astrav1alpha1.DemandShapeDecodeHeavy: {
			astrav1alpha1.ResourceDecodeTokensPerSec: 0.4,
		},
		astrav1alpha1.DemandShapePrefillHeavy: {
			astrav1alpha1.ResourcePrefillTokensPerSec: 0.4,
		},
		astrav1alpha1.DemandShapeKVHeavy: {
			astrav1alpha1.ResourceKVCacheGiB: 0.4,
		},
		astrav1alpha1.DemandShapeMixed: {
			astrav1alpha1.ResourceBalancedResourceFocus: 0.4,
		},
	}
}

func focusedResourceForDemandShape(demandShape astrav1alpha1.DemandShape) astrav1alpha1.ResourceName {
	return DefaultDemandShapeResourceMapping().FocusedResource(demandShape)
}

func (m DemandShapeResourceMapping) FocusedResource(demandShape astrav1alpha1.DemandShape) astrav1alpha1.ResourceName {
	weights := m[normalizedDemandShape(demandShape)]
	if len(weights) == 0 {
		return astrav1alpha1.ResourceBalancedResourceFocus
	}

	var bestResource astrav1alpha1.ResourceName
	var bestWeight float64
	for resource, weight := range weights {
		if weight > bestWeight {
			bestResource = resource
			bestWeight = weight
		}
	}
	if bestResource == "" {
		return astrav1alpha1.ResourceBalancedResourceFocus
	}
	return bestResource
}

func resourceHeadroomWeight(key, resourceName string) float64 {
	if key == string(astrav1alpha1.ResourceBalancedResourceFocus) || key == "" {
		return 0.4
	}
	if resourceName == key {
		return 0.4
	}
	return 0.3
}
