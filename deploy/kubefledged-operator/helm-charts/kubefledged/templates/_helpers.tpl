{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "kubefledged.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "kubefledged.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "kubefledged.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels
*/}}
{{- define "kubefledged.labels" -}}
helm.sh/chart: {{ include "kubefledged.chart" . }}
{{ include "kubefledged.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/*
Selector labels
*/}}
{{- define "kubefledged.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kubefledged.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: {{ .Release.Name }}
{{- end -}}

{{/*
Create the name of the service account to use
*/}}
{{- define "kubefledged.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
    {{ default (include "kubefledged.fullname" .) .Values.serviceAccount.name }}
{{- else -}}
    {{ default "default" .Values.serviceAccount.name }}
{{- end -}}
{{- end -}}

{{/*
Create the name of the cluster role to use
*/}}
{{- define "kubefledged.clusterRoleName" -}}
{{- if .Values.clusterRole.create -}}
    {{ default (include "kubefledged.fullname" .) .Values.clusterRole.name }}
{{- else -}}
    {{ default "default" .Values.clusterRole.name }}
{{- end -}}
{{- end -}}

{{/*
Create the name of the cluster role binding to use
*/}}
{{- define "kubefledged.clusterRoleBindingName" -}}
{{- if .Values.clusterRoleBinding.create -}}
    {{ default (include "kubefledged.fullname" .) .Values.clusterRoleBinding.name }}
{{- else -}}
    {{ default "default" .Values.clusterRoleBinding.name }}
{{- end -}}
{{- end -}}

{{/*
Create the name of the validating webhook configuration to use
*/}}
{{- define "kubefledged.validatingWebhookName" -}}
{{- if .Values.validatingWebhook.create -}}
    {{ default (include "kubefledged.fullname" .) .Values.validatingWebhook.name }}
{{- else -}}
    {{ default "default" .Values.validatingWebhook.name }}
{{- end -}}
{{- end -}}

{{/*
Create the name of the service for the webhook server to use
*/}}
{{- define "kubefledged.webhookServiceName" -}}
{{- if .Values.webhookService.create -}}
    {{ default ( printf "%s-webhook-server" (include "kubefledged.fullname" .)) .Values.webhookService.name }}
{{- else -}}
    {{ default "default" .Values.webhookService.name }}
{{- end -}}
{{- end -}}

{{/*
Create the name of the secret containing the webhook server's keypair
*/}}
{{- define "kubefledged.secretName" -}}
{{ default (include "kubefledged.fullname" .) .Values.secret.name }}
{{- end -}}
