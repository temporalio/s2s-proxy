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
The proxy's config file. Helm merges .Values itself, so this only assembles what the file needs:
everything under config, plus clusterConnections turned from a keyed map into the list the proxy
reads. Each entry is merged over clusterConnectionDefaults.
*/}}
{{- define "s2s-proxy.config" -}}
{{- if not .Values.clusterConnections }}
    {{- fail "clusterConnections is empty. Set at least one, keyed by name. See values.example.yaml." }}
{{- end }}
{{- $config := deepCopy .Values.config }}
{{- $connections := list }}
{{- range $name, $connection := .Values.clusterConnections }}
    {{- $merged := deepCopy $.Values.clusterConnectionDefaults | merge (deepCopy $connection) }}
    {{- $_ := set $merged "name" $name }}
    {{- $connections = append $connections $merged }}
{{- end }}
{{- $_ := set $config "clusterConnections" $connections }}
{{- $config | toYaml }}
{{- end }}

{{/*
The ports the container listens on, read back out of the rendered config.

Every cluster connection binds its own egress port, so they are returned as a list. The health and
metrics listeners are per process.
*/}}
{{- define "s2s-proxy.parsedPorts" -}}
{{- $config := (include "s2s-proxy.config" . | fromYaml) }}

{{- $egressPorts := list }}
{{- $healthPorts := list }}
{{- range $config.clusterConnections }}
    {{- $egressPorts = append $egressPorts (split ":" .local.tcpServer.address)._1 }}
    {{- $healthPorts = append $healthPorts (split ":" .remoteClusterHealthCheck.listenAddress)._1 }}
{{- end }}

{{- $metricsPort := (split ":" $config.metrics.prometheus.listenAddress)._1 }}

{{- dict "egress" (uniq $egressPorts) "health" (uniq $healthPorts) "metrics" $metricsPort | toYaml }}
{{- end }}
