{{- define "kronk.fullname" -}}
{{- /* Leave nine characters for the longest suffix, -headless. */ -}}
{{- if contains .Chart.Name .Release.Name -}}
{{- .Release.Name | trunc 54 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 54 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "kronk.selectorLabels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "kronk.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{ include "kronk.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "kronk.image" -}}
{{- printf "%s:%s" .Values.image.repository (default (printf "v%s-%s" .Chart.AppVersion .Values.image.variant) .Values.image.tag) -}}
{{- end -}}

{{- define "kronk.extraEnv" -}}
{{- $reserved := list "KRONK_BASE_PATH" "KRONK_POOL_MODEL_CONFIG_FILE" "KRONK_WEB_API_HOST" "KRONK_WEB_DEBUG_HOST" "KRONK_PROCESSOR" "KRONK_AUTHORIZATION_MODE" -}}
{{- $seen := dict -}}
{{- range $name, $value := .Values.env -}}
{{- if has $name $reserved -}}
{{- fail (printf "env: %s is chart-owned" $name) -}}
{{- end -}}
{{- $_ := set $seen $name true }}
- name: {{ $name }}
  value: {{ $value | quote }}
{{- end -}}
{{- range .Values.extraEnv -}}
{{- if or (has .name $reserved) (hasKey $seen .name) -}}
{{- fail (printf "extraEnv: %s is chart-owned or duplicated" .name) -}}
{{- end -}}
{{- $_ := set $seen .name true }}
{{ toYaml (list .) }}
{{- end -}}
{{- end -}}
