{{/*
Expand the name of the chart.
*/}}
{{- define "doks-lb-scale.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "doks-lb-scale.fullname" -}}
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
{{- define "doks-lb-scale.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "doks-lb-scale.labels" -}}
helm.sh/chart: {{ include "doks-lb-scale.chart" . }}
{{ include "doks-lb-scale.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "doks-lb-scale.selectorLabels" -}}
app.kubernetes.io/name: {{ include "doks-lb-scale.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "doks-lb-scale.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "doks-lb-scale.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the secret to use
*/}}
{{- define "doks-lb-scale.secretName" -}}
{{- .Values.config.doApiTokenSecret }}
{{- end }}

{{/*
Create the name of the deployment to use
*/}}
{{- define "doks-lb-scale.deploymentName" -}}
{{- include "doks-lb-scale.fullname" . }}
{{- end }}

{{/*
Create the name of the cluster role to use
*/}}
{{- define "doks-lb-scale.clusterRoleName" -}}
{{- include "doks-lb-scale.fullname" . }}
{{- end }}

{{/*
Create the name of the cluster role binding to use
*/}}
{{- define "doks-lb-scale.clusterRoleBindingName" -}}
{{- include "doks-lb-scale.fullname" . }}
{{- end }}

{{/*
Create the name of the pod disruption budget to use
*/}}
{{- define "doks-lb-scale.podDisruptionBudgetName" -}}
{{- include "doks-lb-scale.fullname" . }}
{{- end }}

{{/*
Get the image registry
*/}}
{{- define "doks-lb-scale.imageRegistry" -}}
{{- if and .Values.global .Values.global.imageRegistry }}
{{- .Values.global.imageRegistry }}
{{- else if .Values.image.registry }}
{{- .Values.image.registry }}
{{- end }}
{{- end }}

{{/*
Get the image pull policy
*/}}
{{- define "doks-lb-scale.imagePullPolicy" -}}
{{- if and .Values.global .Values.global.imagePullPolicy }}
{{- .Values.global.imagePullPolicy }}
{{- else if .Values.image.pullPolicy }}
{{- .Values.image.pullPolicy }}
{{- else }}
{{- "IfNotPresent" }}
{{- end }}
{{- end }}

{{/*
Get the full image name
*/}}
{{- define "doks-lb-scale.image" -}}
{{- $registry := include "doks-lb-scale.imageRegistry" . }}
{{- $repository := .Values.image.repository }}
{{- $tag := .Values.image.tag | default .Chart.AppVersion }}
{{- if $registry }}
{{- printf "%s/%s:%s" $registry $repository $tag }}
{{- else }}
{{- printf "%s:%s" $repository $tag }}
{{- end }}
{{- end }}

{{/*
Create the namespace for leader election
*/}}
{{- define "doks-lb-scale.leaderElectionNamespace" -}}
{{- if .Values.leaderElection.namespace }}
{{- .Values.leaderElection.namespace }}
{{- else }}
{{- .Release.Namespace }}
{{- end }}
{{- end }}

{{/*
Create the leader election ID
*/}}
{{- define "doks-lb-scale.leaderElectionId" -}}
{{- if .Values.leaderElection.id }}
{{- .Values.leaderElection.id }}
{{- else }}
{{- include "doks-lb-scale.fullname" . }}
{{- end }}
{{- end }}
