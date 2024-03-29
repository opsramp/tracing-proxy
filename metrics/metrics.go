package metrics

import (
	"github.com/opsramp/tracing-proxy/types"
	"strings"
)

type Metrics interface {
	// Register declares a metric; metricType should be one of counter, gauge, histogram
	Register(name string, metricType string)
	Increment(name string)
	Gauge(name string, val interface{})
	Count(name string, n interface{})
	Histogram(name string, obs interface{})
	RegisterWithDescriptionLabels(name string, metricType string, desc string, labels []string)
	RegisterGauge(name string, labels []string, desc string)
	RegisterCounter(name string, labels []string, desc string)
	RegisterHistogram(name string, labels []string, desc string, buckets []float64)

	GaugeWithLabels(name string, labels map[string]string, value float64)
	IncrementWithLabels(name string, labels map[string]string)
	AddWithLabels(name string, labels map[string]string, value float64)
	HistogramWithLabels(name string, labels map[string]string, obs interface{})
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

func ExtractLabelsFromSpan(span *types.Span, labelToKeyMap map[string][]string) map[string]string {
	labels := map[string]string{}

	attributeMapKeys := []string{"spanAttributes", "resourceAttributes", "eventAttributes"}

	for labelName, searchKeys := range labelToKeyMap {
		var searchKeyExists bool
		for _, searchKey := range searchKeys {
			// check of the higher level first
			searchValue, exists := span.Data[searchKey]
			if exists && searchValue != nil {
				if val, ok := searchValue.(string); ok {
					val = simplifySpanName(val)
					labels[labelName] = val
					searchKeyExists = true
				}
				break
			}

			// check in the span, resource and event attributes when key is not found
			for _, attributeKey := range attributeMapKeys {
				if attribute, ok := span.Data[attributeKey]; ok && attribute != nil {
					searchValue, exists = attribute.(map[string]interface{})[searchKey]
					if exists && searchValue != nil {
						if val, ok := searchValue.(string); ok {
							labels[labelName] = val
							searchKeyExists = true
						}
						break
					}
				}
			}

			// if the key does not exist then set it to empty
			if !exists && !searchKeyExists {
				labels[labelName] = ""
			}
		}
	}

	return labels
}

func simplifySpanName(input string) string {

	regexCheck := ""
	var haveNumber, haveChar bool
	changedString := input
	for k, i := range input {
		//fmt.Println(k, len(input))

		if string(i) == "|" || string(i) == "_" || string(i) == "-" || string(i) == ":" || string(i) == "/" || k == len(input)-1 {

			//fmt.Println(haveNumber, haveChar)
			if k == len(input)-1 {
				regexCheck = regexCheck + string(i)
				if string(i) >= "a" {
					//fmt.Println(regexCheck)
					haveChar = true
				} else if i >= 48 && i <= 57 {
					//fmt.Println(i, string(i))
					haveNumber = true
				}
			}

			if haveNumber && haveChar {
				//fmt.Println(matches[0])
				//fmt.Println(regexCheck)

				changedString = strings.Replace(changedString, regexCheck, "*", 1)
			}
			regexCheck = ""
			haveNumber = false
			haveChar = false
		} else {
			regexCheck = regexCheck + string(i)
			if string(i) >= "a" {
				//fmt.Println(regexCheck)
				haveChar = true
			} else if i >= 48 && i <= 57 {
				//fmt.Println(i, string(i))
				haveNumber = true
			}
		}
	}
	return changedString
}
