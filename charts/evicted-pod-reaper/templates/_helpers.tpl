{{/*
Expand the name of the chart.
*/}}
{{- define "evicted-pod-reaper.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "evicted-pod-reaper.fullname" -}}
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
{{- define "evicted-pod-reaper.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "evicted-pod-reaper.labels" -}}
helm.sh/chart: {{ include "evicted-pod-reaper.chart" . }}
{{ include "evicted-pod-reaper.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- with .Values.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "evicted-pod-reaper.selectorLabels" -}}
app.kubernetes.io/name: {{ include "evicted-pod-reaper.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "evicted-pod-reaper.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "evicted-pod-reaper.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the namespace list for watching
*/}}
{{- define "evicted-pod-reaper.watchNamespaces" -}}
{{- if .Values.reaper.watchAllNamespaces -}}
{{- else -}}
{{- join "," .Values.reaper.watchNamespaces -}}
{{- end -}}
{{- end }}

{{/*
Determine RBAC type (ClusterRole or Role)
*/}}
{{- define "evicted-pod-reaper.rbacKind" -}}
{{- if .Values.reaper.watchAllNamespaces -}}
ClusterRole
{{- else -}}
Role
{{- end -}}
{{- end }}

{{/*
Create environment variables for the controller
*/}}
{{- define "evicted-pod-reaper.envVars" -}}
- name: REAPER_WATCH_ALL_NAMESPACES
  value: {{ .Values.reaper.watchAllNamespaces | quote }}
{{- if not .Values.reaper.watchAllNamespaces }}
- name: REAPER_WATCH_NAMESPACES
  value: {{ include "evicted-pod-reaper.watchNamespaces" . | quote }}
{{- end }}
- name: REAPER_TTL_TO_DELETE
  value: {{ .Values.reaper.ttlToDelete | quote }}
{{- with .Values.reaper.env }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Common annotations
*/}}
{{- define "evicted-pod-reaper.annotations" -}}
{{- with .Values.commonAnnotations }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Get the container image
*/}}
{{- define "evicted-pod-reaper.image" -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion -}}
{{- printf "%s:%s" .Values.image.repository $tag -}}
{{- end }}

{{/*
Get leader election ID
*/}}
{{- define "evicted-pod-reaper.leaderElectionID" -}}
{{- printf "%s.kyos.io" (include "evicted-pod-reaper.fullname" .) -}}
{{- end }}
