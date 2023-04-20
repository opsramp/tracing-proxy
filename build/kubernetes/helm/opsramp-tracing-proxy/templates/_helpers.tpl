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
Service Ports
*/}}
{{- define "httpPort" -}}
{{ if .Values.service }} {{ default 8082 .Values.service.http }} {{ else }} 8082 {{ end }}
{{- end }}
{{- define "grpcPort" -}}
{{ if .Values.service }} {{ default 9090 .Values.service.grpc }} {{ else }} 9090 {{ end }}
{{- end }}
{{- define "httpPeerPort" -}}
{{ if .Values.service }} {{ default 8081 .Values.service.peer }} {{ else }} 8081 {{ end }}
{{- end }}
{{- define "grpcPeerPort" -}}
{{ if .Values.service }} {{ default 8084 .Values.service.grpcPeer }} {{ else }} 8084 {{ end }}
{{- end }}


{{/*
Image Defaults
*/}}
{{- define "imagePullPolicy" -}}
{{ if .Values.image }} {{ default "Always" .Values.image.pullPolicy | quote }} {{ else }} "Always" {{ end }}
{{- end }}


{{/*
Redis Defaults
*/}}
{{- define "opsramp-tracing-proxy.redis.fullname" -}}
{{ include "opsramp-tracing-proxy.fullname" . }}-redis
{{- end }}
{{- define "opsramp-tracing-proxy.redis.labels" -}}
helm.sh/chart: {{ include "opsramp-tracing-proxy.chart" . }}
{{ include "opsramp-tracing-proxy.redis.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}
{{- define "opsramp-tracing-proxy.redis.selectorLabels" -}}
app.kubernetes.io/name: {{ include "opsramp-tracing-proxy.name" . }}-redis
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}