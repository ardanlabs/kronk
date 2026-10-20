{{- define "kronk.fullname" -}}
{{- if contains .Chart.Name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{- define "kronk.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{ include "kronk.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "kronk.selectorLabels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Image reference: v<appVersion>-<variant> unless image.tag is set.
*/}}
{{- define "kronk.image" -}}
{{- $variant := .Values.image.variant -}}
{{- if not (has $variant (list "cpu" "cuda" "vulkan" "rocm" "all")) -}}
{{- fail (printf "image.variant must be one of cpu, cuda, vulkan, rocm, all (got %q)" $variant) -}}
{{- end -}}
{{- $tag := default (printf "v%s-%s" .Chart.AppVersion $variant) .Values.image.tag -}}
{{- printf "%s:%s" .Values.image.repository $tag -}}
{{- end }}

{{- define "kronk.keysSecretName" -}}
{{- default (printf "%s-keys" (include "kronk.fullname" .)) .Values.auth.existingSecret }}
{{- end }}

{{- define "kronk.keysEnabled" -}}
{{- if or .Values.auth.existingSecret .Values.auth.masterPem .Values.auth.masterJwt -}}
true
{{- end -}}
{{- end }}
