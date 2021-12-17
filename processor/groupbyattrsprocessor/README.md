# Group by Attributes processor

Supported pipeline types: **traces**, **logs**, **metrics**

## Description

This processor re-associates spans, log records and metric datapoints to a *Resource* that matches with the specified attributes. As a result, all spans, log records or metric datapoints with the same values for the specified attributes are "grouped" under the same *Resource*.

Typical use cases:

* extract resources from "flat" data formats, such as Fluentbit logs or Prometheus metrics
* associate Prometheus metrics to a *Resource* that describes the relevant host, based on label present on all metrics
* optimize data packaging by extracting common attributes

## Example

Consider the below metrics, all originally associated to the same *Resource*:

```go
Resource {host.name="localhost",source="prom"}
  Metric "gauge-1" (GAUGE)
    DataPoint {host.name="host-A",id="eth0"}
    DataPoint {host.name="host-A",id="eth0"}
    DataPoint {host.name="host-B",id="eth0"}
  Metric "gauge-1" (GAUGE) // Identical to previous Metric
    DataPoint {host.name="host-A",id="eth0"}
    DataPoint {host.name="host-A",id="eth0"}
    DataPoint {host.name="host-B",id="eth0"}
  Metric "mixed-type" (GAUGE)
    DataPoint {host.name="host-A",id="eth0"}
    DataPoint {host.name="host-A",id="eth0"}
    DataPoint {host.name="host-B",id="eth0"}
  Metric "mixed-type" (SUM)
    DataPoint {host.name="host-A",id="eth0"}
    DataPoint {host.name="host-A",id="eth0"}
  Metric "dont-move" (Gauge)
    DataPoint {id="eth0"}
```

With the below configuration, the **groupbyattrs** will re-associate the metrics with either `host-A` or `host-B`, based on the value of the `host.name` attribute.

```yaml
processors:
  groupbyattrs:
    keys:
      - host.name
```

The output of the processor will therefore be:

```go
Resource {host.name="localhost",source="prom"}
  Metric "dont-move" (Gauge)
    DataPoint {id="eth0"}

Resource {host.name="host-A",source="prom"}
  Metric "gauge-1"
    DataPoint {id="eth0"}
    DataPoint {id="eth0"}
    DataPoint {id="eth0"}
    DataPoint {id="eth0"}
  Metric "mixed-type" (GAUGE)
    DataPoint {id="eth0"}
    DataPoint {id="eth0"}
  Metric "mixed-type" (SUM)
    DataPoint {id="eth0"}
    DataPoint {id="eth0"}

Resource {host.name="host-B",source="prom"}
  Metric "gauge-1"
    DataPoint {id="eth0"}
    DataPoint {id="eth0"}
  Metric "mixed-type" (GAUGE)
    DataPoint {id="eth0"}
```

Notes:

* The *DataPoints* for the `gauge-1` (GAUGE) metric were originally split under 2 *Metric* instances and have been merged in the output
* The *DataPoints* of the `mixed-type` (GAUGE) and `mixed-type` (SUM) metrics have not been merged under the same *Metric*, because their *DataType* is different
* The `dont-move` metric *DataPoints* don't have a `host.name` attribute and therefore remained under the original *Resource*
* The new *Resources* inherited the attributes from the original *Resource* (`source="prom"`), **plus** the specified attributes from the processed metrics (`host.name="host-A"` or `host.name="host-B"`)
* The specified "grouping" attributes that are set on the new *Resources* are also **removed** from the metric *DataPoints*
* While not shown in the above example, the processor also merges collections of records under matching InstrumentationLibrary

## Configuration

The configuration is very simple, as you only need to specify an array of attribute keys that will be used to "group" spans, log records or metric data points together, as in the below example:

```yaml
processors:
  groupbyattrs:
    keys:
      - foo
      - bar
```

The `keys` property describes which attribute keys will be considered for grouping:

* If the processed span, log record and metric data point has at least one of the specified attributes key, it will be moved to a *Resource* with the same value for these attributes. The *Resource* will be created if none exists with the same attributes.
* If none of the specified attributes key is present in the processed span, log record or metric data point, it remains associated to the same *Resource* (no change).

Please refer to:

* [config.go](./config.go) for the config spec
* [config.yaml](./testdata/config.yaml) for detailed examples on using the processor

## Internal Metrics

The following internal metrics are recorded by this processor:

| Metric                    | Description                                              |
|---------------------------|----------------------------------------------------------|
| `num_grouped_spans`       | the number of spans that had attributes grouped          |
| `num_non_grouped_spans`   | the number of spans that did not have attributes grouped |
| `span_groups`             | distribution of groups extracted for spans               |
| `num_grouped_logs`        | number of logs that had attributes grouped               |
| `num_non_grouped_logs`    | number of logs that did not have attributes grouped      |
| `log_groups`              | distribution of groups extracted for logs                |
| `num_grouped_metrics`     | number of metrics that had attributes grouped            |
| `num_non_grouped_metrics` | number of metrics that did not have attributes grouped   |
| `metric_groups`           | distribution of groups extracted for metrics             |