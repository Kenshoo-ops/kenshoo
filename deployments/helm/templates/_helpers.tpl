{{- define "kenshoo.labels" -}}
app.kubernetes.io/name: kenshoo
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end }}

{{- define "kenshoo.image" -}}
{{- $d := required "image.digest is required: deploy by digest, never by tag" .Values.image.digest -}}
{{- if not (hasPrefix "sha256:" $d) }}{{ fail "image.digest must look like sha256:..." }}{{ end -}}
{{ .Values.image.repository }}@{{ $d }}
{{- end }}
