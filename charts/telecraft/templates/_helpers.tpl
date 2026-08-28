{{/*
Names and labels, on the conventional Helm shape.
*/}}

{{- define "telecraft.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "telecraft.fullname" -}}
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

{{- define "telecraft.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "telecraft.labels" -}}
helm.sh/chart: {{ include "telecraft.chart" . }}
{{ include "telecraft.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "telecraft.selectorLabels" -}}
app.kubernetes.io/name: {{ include "telecraft.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "telecraft.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "telecraft.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
The image reference. A digest pins the bytes and wins over a tag; a tag
that nobody set is the chart's appVersion, so the chart and the image it
deploys carry one number (ADR-0068 §3 and §5).
*/}}
{{- define "telecraft.image" -}}
{{- if .Values.image.digest -}}
{{ .Values.image.repository }}@{{ .Values.image.digest }}
{{- else -}}
{{ .Values.image.repository }}:{{ default .Chart.AppVersion .Values.image.tag }}
{{- end -}}
{{- end }}

{{- define "telecraft.syncImage" -}}
{{- $sync := .Values.estate.sync -}}
{{- $repository := required "estate.sync.image.repository names the image the estate containers run: any image with git and a POSIX shell. Set it, or turn estate.sync off and supply the checkout in estate.volume." $sync.image.repository -}}
{{- if $sync.image.tag -}}
{{ $repository }}:{{ $sync.image.tag }}
{{- else -}}
{{ $repository }}
{{- end -}}
{{- end }}

{{/*
The port each endpoint listens on inside the pod. These mirror the flag
defaults in cmd/telecraft/serve.go, which the chart lint reads rather than
repeats, so a port that moves in the command fails there.
*/}}
{{- define "telecraft.httpPort" -}}4321{{- end }}
{{- define "telecraft.opampPort" -}}4320{{- end }}

{{/*
The URL the outside sees. The process holds no certificate in any shape
(ADR-0067 §5), so the scheme follows the terminator in front rather than
anything the pod does. Deriving it from the ingress host is what keeps the
two from drifting into a redirect that returns nowhere (ADR-0068 §5).
*/}}
{{- define "telecraft.externalURL" -}}
{{- if .Values.server.externalUrl -}}
{{ .Values.server.externalUrl }}
{{- else if and .Values.ingress.enabled .Values.ingress.host -}}
{{- if .Values.ingress.tls.enabled -}}
https://{{ .Values.ingress.host }}
{{- else -}}
http://{{ .Values.ingress.host }}
{{- end -}}
{{- end -}}
{{- end }}

{{/*
The environment the estate containers share. The credential reaches git
through a helper and an SSH command, both of which read the projected
files at the point of use, so no secret value is an argument, an image
layer or a line in the pod specification (ADR-0071 §2).
*/}}
{{- define "telecraft.syncEnv" -}}
- name: HOME
  value: /home/git
- name: TELECRAFT_ESTATE_DIR
  value: {{ .Values.estate.mountPath | quote }}
- name: TELECRAFT_ESTATE_REPO
  value: {{ required "estate.sync.repo names the estate repository to clone. Set it, or turn estate.sync off and supply the checkout in estate.volume." .Values.estate.sync.repo | quote }}
- name: TELECRAFT_ESTATE_REF
  value: {{ .Values.estate.sync.ref | quote }}
- name: TELECRAFT_ESTATE_INTERVAL
  value: {{ .Values.estate.sync.intervalSeconds | quote }}
{{- if .Values.estate.sync.credentialSecret }}
- name: TELECRAFT_ESTATE_CREDENTIAL
  value: {{ .Values.estate.sync.credentialPath | quote }}
{{- end }}
{{- with .Values.estate.sync.env }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
What the estate containers mount: the shared checkout, a writable home for
git's configuration, and the credential read-only where there is one.
*/}}
{{- define "telecraft.syncMounts" -}}
- name: estate
  mountPath: {{ .Values.estate.mountPath | quote }}
- name: git-home
  mountPath: /home/git
{{- if .Values.estate.sync.credentialSecret }}
- name: estate-credential
  mountPath: {{ .Values.estate.sync.credentialPath | quote }}
  readOnly: true
{{- end }}
{{- with .Values.estate.sync.extraVolumeMounts }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
The credential preamble both estate containers run. It is shared between
them so the clone and the pull authenticate the same way.

The helper is single-quoted deliberately: git runs it through a shell at
the point of use, so the file is read then rather than at start, and a
credential rewritten in place is picked up by the next fetch with no
restart (ADR-0071 §5).
*/}}
{{- define "telecraft.syncCredential" -}}
if [ -n "${TELECRAFT_ESTATE_CREDENTIAL:-}" ]; then
  if [ -f "$TELECRAFT_ESTATE_CREDENTIAL/ssh-privatekey" ]; then
    ssh_options="-i $TELECRAFT_ESTATE_CREDENTIAL/ssh-privatekey -o IdentitiesOnly=yes"
    if [ -f "$TELECRAFT_ESTATE_CREDENTIAL/known_hosts" ]; then
      ssh_options="$ssh_options -o UserKnownHostsFile=$TELECRAFT_ESTATE_CREDENTIAL/known_hosts"
    fi
    GIT_SSH_COMMAND="ssh $ssh_options"
    export GIT_SSH_COMMAND
  fi
  if [ -f "$TELECRAFT_ESTATE_CREDENTIAL/password" ]; then
    git config --global credential.helper '!f() { echo "username=$(cat "$TELECRAFT_ESTATE_CREDENTIAL/username" 2>/dev/null || echo x-access-token)"; echo "password=$(cat "$TELECRAFT_ESTATE_CREDENTIAL/password")"; }; f'
  fi
fi
{{- end }}
