# Tracing-Proxy - Sampling Proxy For OpenTelemetry Traces

[![Go Report Card](https://goreportcard.com/badge/github.com/opsramp/tracing-proxy)](https://goreportcard.com/report/github.com/opsramp/tracing-proxy)
<p>
<img alt="GitHub go.mod Go version" src="https://img.shields.io/github/go-mod/go-version/opsramp/tracing-proxy">
<a href="https://github.com/opsramp/tracing-proxy/releases"><img alt="GitHub release (latest by date)" src="https://img.shields.io/github/v/release/opsramp/tracing-proxy"></a>
<a href="https://pkg.go.dev/github.com/opsramp/tracing-proxy?tab=doc"><img src="https://godoc.org/github.com/golang/gddo?status.svg" alt="GoDoc"></a>
<img alt="GitHub" src="https://img.shields.io/github/license/opsramp/tracing-proxy">
<img alt="GitHub Workflow Status (with branch)" src="https://img.shields.io/github/actions/workflow/status/opsramp/tracing-proxy/codeql.yml?branch=main&label=CodeQL">
<img alt="GitHub repo size" src="https://img.shields.io/github/repo-size/opsramp/tracing-proxy">
</p>

## Purpose

Tracing-Proxy is a trace-aware sampling proxy. It collects spans emitted by your application, gathers them into traces,
and examines them as a whole. This enables the proxy to make an intelligent sampling decision (whether to keep or
discard) based on the entire trace. Buffering the spans allows you to use fields that might be present in different
spans within the trace to influence the sampling decision. For example, the root span might have HTTP status code,
whereas another span might have information on whether the request was served from a cache. Using this proxy, you can
choose to keep only traces that had a 500 status code and were also served from a cache.

### Minimum configuration

The Tracing-Proxy cluster should have at least 2 servers with 2GB RAM and access to 2 cores each.

Additional RAM and CPU can be used by increasing configuration values to have a larger `CacheCapacity`. The cluster
should be monitored for panics caused by running out of memory and scaled up (with either more servers or more RAM per
server) when they occur.

## Scaling Up

Tracing-Proxy uses bounded queues and circular buffers to manage allocating traces, so even under high volume memory use
shouldn't expand dramatically. However, given that traces are stored in a circular buffer, when the throughput of traces
exceeds the size of the buffer, things will start to go wrong. If you have statistics configured, a counter
named `collector_cache_buffer_overrun` will be incremented each time this happens. The symptoms of this will be that
traces will stop getting accumulated together, and instead spans that should be part of the same trace will be treated
as two separate traces. All traces will continue to be sent (and sampled) but the sampling decisions will be
inconsistent so you'll wind up with partial traces making it through the sampler and it will be very confusing. The size
of the circular buffer is a configuration option named `CacheCapacity`. To choose a good value, you should consider the
throughput of traces (e.g. traces / second started) and multiply that by the maximum duration of a trace (say, 3
seconds), then multiply that by some large buffer (maybe 10x). This will give you good headroom.

Determining the number of machines necessary in the cluster is not an exact science, and is best influenced by watching
for buffer overruns. But for a rough heuristic, count on a single machine using about 2G of memory to handle 5000
incoming events and tracking 500 sub-second traces per second (for each full trace lasting less than a second and an
average size of 10 spans per trace).
