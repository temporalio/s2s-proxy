{{/*
Expand the name of the chart.
*/}}
{{- define "s2s-proxy.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "s2s-proxy.fullname" -}}
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
{{- define "s2s-proxy.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "s2s-proxy.labels" -}}
helm.sh/chart: {{ include "s2s-proxy.chart" . }}
{{ include "s2s-proxy.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "s2s-proxy.selectorLabels" -}}
app.kubernetes.io/name: {{ include "s2s-proxy.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}


{{/*
Merge default config with overrides
*/}}
{{- define "s2s-proxy.mergedConfig" -}}
{{- $defaults := .Files.Get "files/default.yaml" | fromYaml }}
{{- $overrides := .Values.configOverride }}
{{- $merged := deepCopy $defaults -}}

{{/* Merge the clusterConnections list - each override item merges with its matching index in defaults */}}
{{- $mergedClusterConnections := list }}
{{- $overrideClusterConnections := $overrides.clusterConnections | default list }}
{{- range $index, $defaultItem := $defaults.clusterConnections }}
    {{- $overrideItem := dict }}
    {{- if lt $index (len $overrideClusterConnections) }}
        {{- $overrideItem = index $overrideClusterConnections $index }}
    {{- end }}
    {{- $mergedItem := deepCopy $defaultItem | merge $overrideItem }}
    {{- $mergedClusterConnections = append $mergedClusterConnections $mergedItem }}
{{- end }}
{{- $_ := set $merged "clusterConnections" $mergedClusterConnections -}}

{{- $merged | toYaml }}
{{- end }}

{{/*
Parse port numbers from merged config
*/}}
{{- define "s2s-proxy.parsedPorts" -}}
{{- $config := (include "s2s-proxy.mergedConfig" . | fromYaml) }}

{{- $firstCluster := index $config.clusterConnections 0 }}
{{- $egress := $firstCluster.local.tcpServer.address }}
{{- $egressPort := (split ":" $egress)._1 }}
{{- $health := $firstCluster.remoteClusterHealthCheck.listenAddress }}
{{- $healthPort := (split ":" $health)._1 }}

{{- $metrics := ($firstCluster.metrics).prometheus.listenAddress | default $config.metrics.prometheus.listenAddress }}
{{- $metricsPort := (split ":" $metrics)._1 }}

{{- dict "egress" $egressPort "health" $healthPort "metrics" $metricsPort | toYaml }}
{{- end }}
