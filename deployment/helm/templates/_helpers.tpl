{{/*
Expand the name of the chart.
*/}}
{{- define "digital-contracting-service.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "digital-contracting-service.fullname" -}}
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
{{- define "digital-contracting-service.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "digital-contracting-service.labels" -}}
helm.sh/chart: {{ include "digital-contracting-service.chart" . }}
{{ include "digital-contracting-service.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "digital-contracting-service.selectorLabels" -}}
app.kubernetes.io/name: {{ include "digital-contracting-service.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "digital-contracting-service.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "digital-contracting-service.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Normalize a route base path to always start with "/" and never end with "/".
*/}}
{{- define "digital-contracting-service.baseRoutePath" -}}
{{- $base := default "digital-contracting-service" .Values.route.basePath -}}
{{- printf "/%s" (trimAll "/" ($base | toString)) -}}
{{- end }}

{{/*
Resolve PostgreSQL host (explicit override or in-chart default).
*/}}
{{- define "digital-contracting-service.postgresqlHost" -}}
{{- if .Values.serviceDiscovery.postgresqlHost -}}
{{- .Values.serviceDiscovery.postgresqlHost -}}
{{- else if .Values.postgresql.enabled -}}
{{- printf "%s-postgresql" .Release.Name -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end }}

{{/*
Resolve Keycloak host (explicit override or in-chart default).
*/}}
{{- define "digital-contracting-service.keycloakHost" -}}
{{- if .Values.serviceDiscovery.keycloakHost -}}
{{- .Values.serviceDiscovery.keycloakHost -}}
{{- else if .Values.keycloak.enabled -}}
{{- printf "%s-keycloak" .Release.Name -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end }}

{{/*
Resolve NATS host (explicit override or in-chart default).
*/}}
{{- define "digital-contracting-service.natsHost" -}}
{{- if .Values.serviceDiscovery.natsHost -}}
{{- .Values.serviceDiscovery.natsHost -}}
{{- else if .Values.nats.enabled -}}
{{- printf "%s-nats" .Release.Name -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end }}

{{/*
Resolve Keycloak port from explicit override, in-chart service, or scheme defaults.
*/}}
{{- define "digital-contracting-service.keycloakPort" -}}
{{- if .Values.serviceDiscovery.keycloakPort -}}
{{- .Values.serviceDiscovery.keycloakPort -}}
{{- else if .Values.keycloak.enabled -}}
{{- default 8080 .Values.keycloak.service.port -}}
{{- else -}}
443
{{- end -}}
{{- end }}

{{/*
DATABASE_URL override or derived from postgres settings.
*/}}
{{- define "digital-contracting-service.databaseURL" -}}
{{- if .Values.database.url -}}
{{- .Values.database.url -}}
{{- else if include "digital-contracting-service.postgresqlHost" . -}}
{{- $host := include "digital-contracting-service.postgresqlHost" . -}}
{{- $port := default 5432 .Values.database.port -}}
{{- $user := default (default "dcs" .Values.postgresql.auth.username) .Values.database.user -}}
{{- $password := default (default "dcs" .Values.postgresql.auth.password) .Values.database.password -}}
{{- $dbname := default (default "dcs" .Values.postgresql.auth.database) .Values.database.name -}}
{{- $sslmode := default "disable" .Values.database.sslmode -}}
{{- printf "host=%s port=%v user=%s password=%s dbname=%s sslmode=%s" $host $port $user $password $dbname $sslmode -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end }}

{{/*
NATS_URL override or derived from nats settings.
*/}}
{{- define "digital-contracting-service.natsURL" -}}
{{- if .Values.messaging.natsURL -}}
{{- .Values.messaging.natsURL -}}
{{- else if include "digital-contracting-service.natsHost" . -}}
{{- $host := include "digital-contracting-service.natsHost" . -}}
{{- $port := default 4222 .Values.messaging.natsPort -}}
{{- printf "nats://%s:%v" $host $port -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end }}

{{/*
Hydra OAuth2/OIDC issuer (URLs issuer / discovery). Requires hydra.enabled.
*/}}
{{- define "digital-contracting-service.hydraIssuerURL" -}}
{{- if .Values.hydra.enabled -}}
{{- if .Values.hydra.config.selfIssuerURL -}}
{{- .Values.hydra.config.selfIssuerURL -}}
{{- else -}}
{{- printf "http://%s-hydra:%d" .Release.Name (.Values.hydra.service.publicPort | int) -}}
{{- end -}}
{{- end -}}
{{- end }}

{{/*
Hydra admin API base URL (login/consent accept).
*/}}
{{- define "digital-contracting-service.hydraAdminURL" -}}
{{- if .Values.hydra.enabled -}}
{{- printf "http://%s-hydra:%d" .Release.Name (.Values.hydra.service.adminPort | int) -}}
{{- end -}}
{{- end }}

{{/*
Keycloak realm URL for Federated Catalogue integration only.
*/}}
{{- define "digital-contracting-service.fcKeycloakRealmURL" -}}
{{- if .Values.fcKeycloak.realmURL -}}
{{- .Values.fcKeycloak.realmURL -}}
{{- else if .Values.keycloak.enabled -}}
{{- $port := .Values.keycloak.service.port | default 8080 | int -}}
{{- printf "http://%s-keycloak:%d/realms/gaia-x" .Release.Name $port -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end }}

{{/*
API path override or derived default.
*/}}
{{- define "digital-contracting-service.apiPath" -}}
{{- if .Values.paths.api -}}
{{- .Values.paths.api -}}
{{- else -}}
{{- printf "%s/api" (include "digital-contracting-service.baseRoutePath" .) -}}
{{- end -}}
{{- end }}

{{/*
UI path override or derived default.
*/}}
{{- define "digital-contracting-service.uiPath" -}}
{{- if .Values.paths.ui -}}
{{- .Values.paths.ui -}}
{{- else -}}
{{- printf "%s/ui" (include "digital-contracting-service.baseRoutePath" .) -}}
{{- end -}}
{{- end }}

{{/*
Normalize Keycloak route path (leading slash, no trailing slash).
*/}}
{{- define "digital-contracting-service.keycloakRoutePath" -}}
{{- if .Values.keycloak.route.path -}}
{{- printf "/%s" (trimAll "/" (.Values.keycloak.route.path | toString)) -}}
{{- end -}}
{{- end }}
