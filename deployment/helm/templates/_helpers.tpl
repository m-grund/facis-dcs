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
IPFS Document Manager tenant base URL (auto-wired when ipfsDocumentManager sub-chart is enabled).
*/}}
{{- define "digital-contracting-service.ipfsTenantBaseURL" -}}
{{- if .Values.ipfsClient.tenantBaseURL -}}
{{- .Values.ipfsClient.tenantBaseURL -}}
{{- else if .Values.ipfsDocumentManager.enabled -}}
{{- $host := printf "%s-ipfs-document-manager" .Release.Name -}}
{{- $port := default 8080 .Values.ipfsDocumentManager.service.port -}}
{{- $tenant := default "tenant_space" .Values.ipfsClient.tenantName -}}
{{- printf "http://%s:%v/v1/tenants/%s" $host $port $tenant -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end }}

{{/*
CRYPTO_PROVIDER_URL: explicit override, or derived from the co-deployed signer service.
VAULT_ADDR: explicit override, or derived from the co-deployed Vault instance.
*/}}
{{- define "digital-contracting-service.cryptoProviderURL" -}}
{{- if .Values.signing.cryptoProviderURL -}}
{{- .Values.signing.cryptoProviderURL -}}
{{- else if .Values.cryptoProvider.enabled -}}
{{- printf "http://%s-crypto-provider-signer:%v" .Release.Name .Values.cryptoProvider.signer.port -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end }}

{{/*
CRYPTO_PROVIDER_NAMESPACE: explicit override or taken from subchart transit.mount.
*/}}
{{- define "digital-contracting-service.cryptoProviderNamespace" -}}
{{- if .Values.signing.cryptoProviderNamespace -}}
{{- .Values.signing.cryptoProviderNamespace -}}
{{- else if .Values.cryptoProvider.enabled -}}
{{- .Values.cryptoProvider.transit.mount -}}
{{- end -}}
{{- end }}

{{/*
VAULT_ADDR for OID4VP authorization request signing (transit engine).
*/}}
{{- define "digital-contracting-service.vaultAddr" -}}
{{- if .Values.oid4vp.trust.vaultAddr -}}
{{- .Values.oid4vp.trust.vaultAddr -}}
{{- else if .Values.cryptoProvider.enabled -}}
{{- printf "http://%s-crypto-provider-vault:%v" .Release.Name .Values.cryptoProvider.vault.service.port -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end }}

{{/*
IPFS MFS base URL - Kubo RPC API (auto-wired when ipfs sub-chart is enabled).
*/}}
{{- define "digital-contracting-service.ipfsMfsBaseURL" -}}
{{- if .Values.ipfsClient.mfsBaseURL -}}
{{- .Values.ipfsClient.mfsBaseURL -}}
{{- else if .Values.ipfs.enabled -}}
{{- $host := printf "%s-ipfs" .Release.Name -}}
{{- $port := default 5001 .Values.ipfs.service.apiPort -}}
{{- printf "http://%s:%v" $host $port -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end }}

{{/*
CRYPTO_PROVIDER_KEY: explicit override or taken from subchart transit.key.
*/}}
{{- define "digital-contracting-service.cryptoProviderKey" -}}
{{- if .Values.signing.cryptoProviderKey -}}
{{- .Values.signing.cryptoProviderKey -}}
{{- else if .Values.cryptoProvider.enabled -}}
{{- .Values.cryptoProvider.transit.key -}}
{{- end -}}
{{- end }}

{{/*
CRYPTO_PROVIDER_VC_KEY: explicit override or taken from subchart transit.vcKey.
*/}}
{{- define "digital-contracting-service.cryptoProviderVCKey" -}}
{{- if .Values.signing.cryptoProviderVCKey -}}
{{- .Values.signing.cryptoProviderVCKey -}}
{{- else if .Values.cryptoProvider.enabled -}}
{{- .Values.cryptoProvider.transit.vcKey -}}
{{- end -}}
{{- end }}

{{/*
ISSUER_DID: explicit value or secret ref.
*/}}
{{- define "digital-contracting-service.issuerDID" -}}
{{- .Values.signing.issuerDID -}}
{{- end }}

{{/*
Resolve signer cert-chain secret name:
1) explicit signing.certChain existingSecret
2) auto-generated dev cert-chain from co-deployed crypto-provider
*/}}
{{- define "digital-contracting-service.signingCertChainSecretName" -}}
{{- if and .Values.signing.certChain.enabled .Values.signing.certChain.existingSecret.name -}}
{{- .Values.signing.certChain.existingSecret.name -}}
{{- else if and .Values.cryptoProvider.enabled .Values.cryptoProvider.devCertChain.enabled -}}
{{- default (printf "%s-crypto-provider-dev-cert-chain" .Release.Name) .Values.cryptoProvider.devCertChain.secretName -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end }}

{{/*
Resolve signer cert-chain secret key.
*/}}
{{- define "digital-contracting-service.signingCertChainSecretKey" -}}
{{- if and .Values.signing.certChain.enabled .Values.signing.certChain.existingSecret.name -}}
{{- .Values.signing.certChain.existingSecret.key -}}
{{- else if and .Values.cryptoProvider.enabled .Values.cryptoProvider.devCertChain.enabled -}}
{{- default "chain.pem" .Values.cryptoProvider.devCertChain.secretKey -}}
{{- else -}}
{{- "chain.pem" -}}
{{- end -}}
{{- end }}

{{/*
IPFS_MFS_BASE_URL: explicit value or secret ref.
*/}}
{{- define "digital-contracting-service.ipfsMFSBaseURL" -}}
{{- .Values.ipfs.mfsBaseURL -}}
{{- end }}

{{/*
Normalize Keycloak route path (leading slash, no trailing slash).
*/}}
{{- define "digital-contracting-service.keycloakRoutePath" -}}
{{- if .Values.keycloak.route.path -}}
{{- printf "/%s" (trimAll "/" (.Values.keycloak.route.path | toString)) -}}
{{- end -}}
{{- end }}

