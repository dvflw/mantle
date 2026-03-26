{{/*
Expand the name of the chart.
*/}}
{{- define "mantle.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "mantle.fullname" -}}
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
{{- define "mantle.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels.
*/}}
{{- define "mantle.labels" -}}
helm.sh/chart: {{ include "mantle.chart" . }}
{{ include "mantle.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels.
*/}}
{{- define "mantle.selectorLabels" -}}
app.kubernetes.io/name: {{ include "mantle.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use.
*/}}
{{- define "mantle.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "mantle.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Return the database secret name.
*/}}
{{- define "mantle.databaseSecretName" -}}
{{- if .Values.database.existingSecret }}
{{- .Values.database.existingSecret }}
{{- else }}
{{- include "mantle.fullname" . }}
{{- end }}
{{- end }}

{{/*
Return the database secret key.
*/}}
{{- define "mantle.databaseSecretKey" -}}
{{- if .Values.database.existingSecret }}
{{- .Values.database.secretKey }}
{{- else }}
{{- "database-url" }}
{{- end }}
{{- end }}

{{/*
Return the encryption secret name.
*/}}
{{- define "mantle.encryptionSecretName" -}}
{{- if .Values.encryption.existingSecret }}
{{- .Values.encryption.existingSecret }}
{{- else }}
{{- include "mantle.fullname" . }}
{{- end }}
{{- end }}

{{/*
Return the encryption secret key.
*/}}
{{- define "mantle.encryptionSecretKey" -}}
{{- if .Values.encryption.existingSecret }}
{{- .Values.encryption.secretKey }}
{{- else }}
{{- "encryption-key" }}
{{- end }}
{{- end }}
