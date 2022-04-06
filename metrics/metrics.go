package metrics

import (
	"github.com/jirs5/tracing-proxy/types"
)

type Metrics interface {
	// Register declares a metric; metricType should be one of counter, gauge, histogram
	Register(name string, metricType string)
	Increment(name string)
	Gauge(name string, val interface{})
	Count(name string, n interface{})
	Histogram(name string, obs interface{})
	RegisterWithDescriptionLabels(name string, metricType string, desc string, labels []string)

	GaugeWithLabels(name string, labels map[string]string, value float64)
	IncrementWithLabels(name string, labels map[string]string)
}

func GetMetricsImplementation(prefix string) Metrics {
	return &OpsRampMetrics{prefix: prefix}
}

func ConvertNumeric(val interface{}) float64 {
	switch n := val.(type) {
	case int:
		return float64(n)
	case uint:
		return float64(n)
	case int64:
		return float64(n)
	case uint64:
		return float64(n)
	case int32:
		return float64(n)
	case uint32:
		return float64(n)
	case int16:
		return float64(n)
	case uint16:
		return float64(n)
	case int8:
		return float64(n)
	case uint8:
		return float64(n)
	case float64:
		return n
	case float32:
		return float64(n)
	default:
		return 0
	}
}

func ExtractLabelsFromSpan(span *types.Span, labelToKeyMap map[string]string) map[string]string {

	labels := map[string]string{}

	attributeMapKeys := []string{"spanAttributes", "resourceAttributes", "eventAttributes"}

	for labelName, searchKey := range labelToKeyMap {

		// check of the higher level first
		searchValue, exists := span.Data[searchKey]
		if exists && searchValue != nil {
			labels[labelName] = searchValue.(string)
			continue
		}

		// check in the span, resource and event attributes when key is not found
		for _, attributeKey := range attributeMapKeys {
			if attribute, ok := span.Data[attributeKey]; ok && attribute != nil {
				searchValue, exists = attribute.(map[string]interface{})[searchKey]
				if exists && searchValue != nil {
					labels[labelName] = searchValue.(string)
					break
				}
			}
		}

		// if the key does not exist then set it to empty
		if !exists {
			labels[labelName] = ""
		}
	}

	return labels
}
