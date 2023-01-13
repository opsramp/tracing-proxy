{{/*
Expand the name of the chart.
*/}}
{{- define "opsramp-tracing-proxy.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "opsramp-tracing-proxy.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "opsramp-tracing-proxy.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "opsramp-tracing-proxy.labels" -}}
helm.sh/chart: {{ include "opsramp-tracing-proxy.chart" . }}
{{ include "opsramp-tracing-proxy.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "opsramp-tracing-proxy.selectorLabels" -}}
app.kubernetes.io/name: {{ include "opsramp-tracing-proxy.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Service Defaults
*/}}
{{- define "serviceType" -}}
{{ if .Values.service }} {{ default "ClusterIP" .Values.service.type | quote }} {{ else }} "ClusterIP" {{ end }}
{{- end }}
{{- define "servicePort" -}}
{{ if .Values.service }} {{ default 9090 .Values.service.port }} {{ else }} 9090 {{ end }}
{{- end }}

{{/*
Image Defaults
*/}}
{{- define "imagePullPolicy" -}}
{{ if .Values.image }} {{ default "Always" .Values.image.pullPolicy | quote }} {{ else }} "Always" {{ end }}
{{- end }}

{{/*
Config Defautls
*/}}
{{- define "opsrampApiServer" -}}
{{ if .Values.config }} {{ default "" .Values.config.api | quote }} {{ else }} "" {{ end }}
{{- end }}
{{- define "opsrampKey" -}}
{{ if .Values.config }} {{ default "" .Values.config.key | quote }} {{ else }} "" {{ end }}
{{- end }}
{{- define "opsrampSecret" -}}
{{ if .Values.config }} {{ default "" .Values.config.secret | quote }} {{ else }} "" {{ end }}
{{- end }}
{{- define "opsrampTenantId" -}}
{{ if .Values.config }} {{ default "" .Values.config.tenantId | quote }} {{ else }} "" {{ end }}
{{- end }}
{{- define "dataset" -}}
{{ if .Values.config }} {{ default "ds" .Values.config.dataset | quote }} {{ else }} "ds" {{ end }}
{{- end }}
{{- define "useTLS" -}}
{{ if .Values.config }} {{ default true .Values.config.useTls }} {{ else }} true {{ end }}
{{- end }}
{{- define "useTlsInsecure" -}}
{{ if .Values.config }} {{ default false .Values.config.useTlsInsecure }} {{ else }} false {{ end }}
{{- end }}
{{- define "sendMetricsToOpsRamp" -}}
{{ if .Values.config }} {{ default true .Values.config.sendMetricsToOpsRamp }} {{ else }} true {{ end }}
{{- end }}
{{- define "proxyProtocol" -}}
{{ if and .Values.config .Values.config.proxy }} {{ default "" $.Values.config.proxy.protocol | quote }} {{ else }} "" {{ end }}
{{- end }}
{{- define "proxyServer" -}}
{{ if and .Values.config .Values.config.proxy }} {{ default "" $.Values.config.proxy.server | quote }} {{ else }} "" {{ end }}
{{- end }}
{{- define "proxyPort" -}}
{{ if and .Values.config .Values.config.proxy }} {{ default 3128 $.Values.config.proxy.port }} {{ else }} 3128 {{ end }}
{{- end }}
{{- define "proxyUsername" -}}
{{ if and .Values.config .Values.config.proxy }} {{ default "" $.Values.config.proxy.username | quote }} {{ else }} "" {{ end }}
{{- end }}
{{- define "proxyPassword" -}}
{{ if and .Values.config .Values.config.proxy }} {{ default "" $.Values.config.proxy.password | quote }} {{ else }} "" {{ end }}
{{- end }}

{{/*
Logging Defautls
*/}}
{{- define "logFormat" -}}
{{ if .Values.logging }} {{ default "json" .Values.logging.logFormat | quote }} {{ else }} "json" {{ end }}
{{- end }}
{{- define "logOutput" -}}
{{ if .Values.logging }} {{ default "stdout" .Values.logging.logOutput | quote }} {{ else }} "stdout" {{ end }}
{{- end }}

{{/*
Metrics Defaults
*/}}
{{- define "metricsList" -}}
{{ if .Values.metrics }} {{ default `[".*"]` .Values.metrics.list }} {{ else }} [".*"] {{ end }}
{{- end }}