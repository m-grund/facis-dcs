SHELL := /bin/bash -eu -o pipefail

PYTHON ?= python3
# Unter WSL gegen /mnt/c liegt das Repo auf dem 9P-Dateisystem — ein venv dort
# macht jeden Python-Import 5-20x langsamer. Default daher: venv im nativen
# ext4-Home der Distro. Explizites VENV_PATH=... überschreibt weiterhin;
# außerhalb von WSL (CI-Runner, macOS) bleibt der bisherige Default .venv.
ifneq ($(wildcard /proc/sys/fs/binfmt_misc/WSLInterop),)
VENV_PATH ?= $(HOME)/.dcs-bdd-venv
endif
VENV_PATH ?= .venv
THIS_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
PROJECT_ROOT := $(abspath $(THIS_DIR)../..)
VENV_ABS := $(if $(filter /%,$(VENV_PATH)),$(VENV_PATH),$(THIS_DIR)$(VENV_PATH))
REPORTS_ABS := $(THIS_DIR).reports
BDD_EXECUTOR_REPO ?= https://github.com/eclipse-xfsc/bdd-executor.git
BDD_EXECUTOR_PATH ?= ../../bdd-executor
BDD_EXECUTOR_FALLBACK_PATH ?= .tmp/bdd-executor
FEATURES_PATH ?= features
ARG_BDD ?=
ARG_BDD_JUNIT ?= --junit --junit-directory $(REPORTS_ABS)/junit
K8S_NAMESPACE ?= default
HELM_RELEASE ?= dcs
DCS_SERVICE ?= $(HELM_RELEASE)-digital-contracting-service
DCS_DEPLOYMENT ?= $(DCS_SERVICE)
LOCAL_FORWARD_PORT ?= 18991
SERVICE_PORT ?= 8991
DCS_DID ?= localhost
DCS_API_BASE_PATH ?= /digital-contracting-service/api
BDD_INGRESS_PORT ?= 18080
BDD_PUBLIC_ORIGIN ?= http://localhost:$(BDD_INGRESS_PORT)
BDD_DCS_BASE_URL ?= $(BDD_PUBLIC_ORIGIN)$(DCS_API_BASE_PATH)
KIND_CONFIG ?= $(THIS_DIR)kind-config.yaml
BDD_HELM_SET := \
	--set-string hydra.config.selfIssuerURL="$(BDD_PUBLIC_ORIGIN)" \
	--set-string route.publicBaseURL="$(BDD_DCS_BASE_URL)" \
	--set-string hydra.config.loginURL="$(BDD_PUBLIC_ORIGIN)/digital-contracting-service/ui/" \
	--set-string hydra.config.consentURL="$(BDD_PUBLIC_ORIGIN)/digital-contracting-service/api/auth/consent" \
	--set-string hydra.config.logoutURL="$(BDD_PUBLIC_ORIGIN)/digital-contracting-service/api/auth/logout-complete" \
	--set-string hydra.clients[0].redirect_uris[0]="$(BDD_PUBLIC_ORIGIN)/digital-contracting-service/api/auth/callback" \
	--set-string hydra.clients[0].post_logout_redirect_uris[0]="$(BDD_PUBLIC_ORIGIN)/digital-contracting-service/api/auth/logout-complete"
HELM_CHART_PATH ?= ../../deployment/helm
HELM_TIMEOUT ?= 20m

KIND_CLUSTER_NAME ?= dcs-bdd
KIND_WAIT ?= 120s
DOCKER_IMAGE_REPOSITORY ?= digital-contracting-service
DOCKER_IMAGE_TAG ?= bdd
DOCKER_IMAGE ?= $(DOCKER_IMAGE_REPOSITORY):$(DOCKER_IMAGE_TAG)
PDF_CORE_IMAGE_REPOSITORY ?= pdf-core
PDF_CORE_IMAGE_TAG ?= bdd
PDF_CORE_IMAGE ?= $(PDF_CORE_IMAGE_REPOSITORY):$(PDF_CORE_IMAGE_TAG)
KIND_NAMESPACE ?= dcs-bdd
KIND_HELM_VALUES ?=
KIND_HELM_SET ?=
KIND_BIN ?= kind
KUBECTL_BIN ?= kubectl
HELM_BIN ?= helm
BDD_RUNNER_SCRIPT ?= scripts/run_bdd_helm.sh
CERT_CHAIN_SECRET ?= $(DCS_SERVICE)-hsm-c2pa-x5chain

.PHONY: setup_environment ensure_venv run_bdd_test run_bdd_fast run_bdd_helm  ensure_kind ensure_k8s_tools kind_create kind_delete kind_load_image kind_load_pdf_core_image kind_deploy kind_uninstall run_bdd_kind run_bdd_kind_ci build_dcs_image clean help kind_up run_bdd_kind_once kind_down

help:
	@echo "make setup_environment   # Create venv and install bdd-executor"
	@echo "make run_bdd_test		# Run all BDD scenarios (coverage + junit reports)"
	@echo "make run_bdd_fast T=@tag	# Fast dev loop: tag-scoped, --stop, no coverage/junit"
	@echo "make run_bdd_helm		# Run BDD via Traefik on http://localhost:18080"
	@echo "make run_bdd_kind		# Build ephemeral kind+Helm stack and run all scenarios"
	@echo "make ensure_kind			# Validate kind is available in PATH"
	@echo "make ensure_k8s_tools	# Validate kubectl and helm are available in PATH"
	@echo "make kind_delete			# Delete kind cluster used for BDD"
	@echo "make clean				# Remove local test artifacts"

setup_environment:
	@if [[ -f "$(VENV_PATH)/bin/pip" ]] && ! "$(VENV_PATH)/bin/pip" --version >/dev/null 2>&1; then \
		echo "Existing virtualenv at $(VENV_PATH) is not usable in this environment; recreating"; \
		rm -rf "$(VENV_PATH)"; \
	fi
	@if [[ ! -f "$(VENV_PATH)/bin/activate" ]]; then \
		$(PYTHON) -m venv "$(VENV_PATH)"; \
	fi
	@SRC_PATH="$(BDD_EXECUTOR_PATH)"; \
	if [[ ! -f "$$SRC_PATH/pyproject.toml" ]]; then \
		mkdir -p .tmp; \
		if [[ ! -d "$(BDD_EXECUTOR_FALLBACK_PATH)/.git" ]]; then \
			git clone --depth 1 "$(BDD_EXECUTOR_REPO)" "$(BDD_EXECUTOR_FALLBACK_PATH)"; \
		fi; \
		SRC_PATH="$(BDD_EXECUTOR_FALLBACK_PATH)"; \
	fi; \
	"$(VENV_PATH)/bin/pip" install -q --upgrade pip wheel; \
	"$(VENV_PATH)/bin/pip" install -q -e "$$SRC_PATH[dev]"; \
	"$(VENV_PATH)/bin/pip" install -q psycopg2-binary
	"$(VENV_PATH)/bin/pip" install -q -r "$(PROJECT_ROOT)/testWallet/requirements.txt"

# Bootstrappt das venv nur, wenn es fehlt/kaputt ist — der pip-Refresh bei
# jedem Lauf kostete sonst 10-60s pro Testzyklus.
ensure_venv:
	@"$(VENV_ABS)/bin/python" -c 'import eu.xfsc.bdd.core' 2>/dev/null || \
		$(MAKE) setup_environment

run_bdd_test: ensure_venv
	source "$(VENV_ABS)/bin/activate" && \
	mkdir -p "$(REPORTS_ABS)/junit" && \
	cd "$(PROJECT_ROOT)" && \
	"$(VENV_ABS)/bin/coverage" run --append -m behave $(ARG_BDD_JUNIT) $(FEATURES_PATH) $(ARG_BDD)

# Schneller Dev-Loop für die Implementierungs-Iteration: kein pip-Refresh,
# kein coverage, kein junit; stoppt beim ersten Fehlschlag. Scope über
# T=<tag-expression> (z. B. T=@REQ-mein-slug) und/oder F=<feature-pfad>.
# Der vollständige, berichtende Lauf bleibt run_bdd_test.
run_bdd_fast: ensure_venv
	@cd "$(PROJECT_ROOT)" && \
	"$(VENV_ABS)/bin/behave" --stop $(if $(T),--tags=$(T)) $(if $(F),$(F),$(FEATURES_PATH)) $(ARG_BDD)

run_bdd_helm: setup_environment ensure_k8s_tools
	@VENV_PATH="$(VENV_ABS)" \
		PROJECT_ROOT="$(PROJECT_ROOT)" \
		FEATURES_PATH="$(FEATURES_PATH)" \
		KUBECTL_BIN="$(KUBECTL_BIN)" \
		K8S_NAMESPACE="$(K8S_NAMESPACE)" \
		DCS_DEPLOYMENT="$(DCS_DEPLOYMENT)" \
		BDD_DCS_BASE_URL="$(BDD_DCS_BASE_URL)" \
		BDD_PUBLIC_ORIGIN="$(BDD_PUBLIC_ORIGIN)" \
		DCS_API_BASE_PATH="$(DCS_API_BASE_PATH)" \
		DCS_SERVICE="$(DCS_SERVICE)" \
		LOCAL_FORWARD_PORT="$(LOCAL_FORWARD_PORT)" \
		SERVICE_PORT="$(SERVICE_PORT)" \
		DCS_DID="$(DCS_DID)" \
		DCS_API_BASE_PATH="$(DCS_API_BASE_PATH)" \
		ARG_BDD_JUNIT="$(ARG_BDD_JUNIT)" \
		ARG_BDD="$(ARG_BDD)" \
		bash "$(BDD_RUNNER_SCRIPT)"

build_dcs_image:
	@docker build -f ../../deployment/docker/Dockerfile -t "$(DOCKER_IMAGE)" ../..

build_pdf_core_image:
	@docker build -f ../../pdf-core/Dockerfile -t "$(PDF_CORE_IMAGE)" ../..

ensure_kind:
	@command -v "$(KIND_BIN)" >/dev/null 2>&1 || (echo "kind not found in PATH" && exit 1)

ensure_k8s_tools:
	@command -v "$(KUBECTL_BIN)" >/dev/null 2>&1 || (echo "kubectl not found in PATH" && exit 1)
	@command -v "$(HELM_BIN)" >/dev/null 2>&1 || (echo "helm not found in PATH" && exit 1)

kind_create: ensure_kind ensure_k8s_tools
	@cluster_healthy=false; \
	if "$(KIND_BIN)" get clusters | grep -qx "$(KIND_CLUSTER_NAME)"; then \
		if docker inspect "$(KIND_CLUSTER_NAME)-control-plane" --format '{{.State.Running}}' 2>/dev/null | grep -q true; then \
			echo "kind cluster $(KIND_CLUSTER_NAME) already exists"; \
			cluster_healthy=true; \
		else \
			echo "kind cluster $(KIND_CLUSTER_NAME) exists but control-plane container is not running, recreating..."; \
			"$(KIND_BIN)" delete cluster --name "$(KIND_CLUSTER_NAME)"; \
		fi; \
	fi; \
	if [[ "$$cluster_healthy" == "false" ]]; then \
		"$(KIND_BIN)" create cluster --name "$(KIND_CLUSTER_NAME)" --config "$(KIND_CONFIG)" --wait "$(KIND_WAIT)"; \
	fi

kind_delete: ensure_kind
	@if "$(KIND_BIN)" get clusters | grep -qx "$(KIND_CLUSTER_NAME)"; then \
		"$(KIND_BIN)" delete cluster --name "$(KIND_CLUSTER_NAME)"; \
	else \
		echo "kind cluster $(KIND_CLUSTER_NAME) does not exist"; \
	fi

kind_load_image: kind_create
	@if ! command -v docker >/dev/null 2>&1; then \
		echo "docker is required to load $(DOCKER_IMAGE) into kind"; \
		exit 1; \
	fi
	@docker image inspect "$(DOCKER_IMAGE)" >/dev/null 2>&1 || (echo "Missing local image $(DOCKER_IMAGE). Build it first." && exit 1)
	@"$(KIND_BIN)" load docker-image "$(DOCKER_IMAGE)" --name "$(KIND_CLUSTER_NAME)"

kind_load_pdf_core_image: kind_create
	@if ! command -v docker >/dev/null 2>&1; then \
		echo "docker is required to load $(PDF_CORE_IMAGE) into kind"; \
		exit 1; \
	fi
	@docker image inspect "$(PDF_CORE_IMAGE)" >/dev/null 2>&1 || (echo "Missing local image $(PDF_CORE_IMAGE). Build it first." && exit 1)
	@"$(KIND_BIN)" load docker-image "$(PDF_CORE_IMAGE)" --name "$(KIND_CLUSTER_NAME)"

kind_deploy: kind_load_image kind_load_pdf_core_image
	@"$(HELM_BIN)" repo add prometheus-community https://prometheus-community.github.io/helm-charts --force-update
	@"$(HELM_BIN)" repo add traefik https://traefik.github.io/charts --force-update
	@"$(HELM_BIN)" repo update
	@"$(HELM_BIN)" dependency build "$(HELM_CHART_PATH)"
	@"$(HELM_BIN)" show crds "$(HELM_CHART_PATH)/charts/kube-prometheus-stack-"*.tgz | "$(KUBECTL_BIN)" apply --server-side -f -
	@# DID identity for DCS (DCS_DID is fatal on startup when missing);
	@# mounted via volumes/volumeMounts in values.bdd.yml. The dcs-identity
	@# Secret (did.json) and the signing key itself are both produced by the
	@# hsm-provision-job.yaml post-install hook from the freshly generated
	@# SoftHSM token key — a static did.json fixture would mismatch a random
	@# per-run token key and crash-loop the backend (identity.NewDIDDocument
	@# is fatal on mismatch), so it is deliberately NOT pre-created here.
	@"$(KUBECTL_BIN)" create namespace "$(KIND_NAMESPACE)" --dry-run=client -o yaml | "$(KUBECTL_BIN)" apply -f -
	@# Deploy without --wait: Helm defers post-install hooks until all deployments are
	@# ready, but DCS can't become ready until the devCertChain post-install hook has
	@# created its Secret — a deadlock. We break the cycle by letting helm return as
	@# soon as resources are applied, then waiting for the secret and DCS separately.
	@"$(HELM_BIN)" upgrade --install "$(HELM_RELEASE)" "$(HELM_CHART_PATH)" \
		--namespace "$(KIND_NAMESPACE)" \
		--create-namespace \
		--timeout "$(HELM_TIMEOUT)" \
		--values "$(HELM_CHART_PATH)/values.bdd.yml" \
		--set image.repository="$(DOCKER_IMAGE_REPOSITORY)" \
		--set image.tag="$(DOCKER_IMAGE_TAG)" \
		$(BDD_HELM_SET) \
		$(KIND_HELM_VALUES) \
		$(KIND_HELM_SET)
	@echo "Waiting for cert chain secret $(CERT_CHAIN_SECRET) (created by post-install hook)..."
	@deadline=$$(( $$(date +%s) + 1200 )); \
	until "$(KUBECTL_BIN)" get secret "$(CERT_CHAIN_SECRET)" -n "$(KIND_NAMESPACE)" >/dev/null 2>&1; do \
		if [ $$(date +%s) -gt $$deadline ]; then \
			echo "Timed out waiting for $(CERT_CHAIN_SECRET)"; \
			"$(KUBECTL_BIN)" get pods -n "$(KIND_NAMESPACE)" -o wide || true; \
			"$(KUBECTL_BIN)" get events -n "$(KIND_NAMESPACE)" --sort-by=.lastTimestamp || true; \
			exit 1; \
		fi; \
		sleep 5; \
	done
	@echo "Waiting for DCS deployment to become available..."
	@if ! "$(KUBECTL_BIN)" -n "$(KIND_NAMESPACE)" rollout status \
		deployment/"$(DCS_DEPLOYMENT)" --timeout "$(HELM_TIMEOUT)"; then \
		echo "=== PODS ==="; \
		"$(KUBECTL_BIN)" get pods -n "$(KIND_NAMESPACE)" -o wide || true; \
		echo "=== DEPLOYMENTS ==="; \
		"$(KUBECTL_BIN)" get deployments -n "$(KIND_NAMESPACE)" || true; \
		echo "=== EVENTS ==="; \
		"$(KUBECTL_BIN)" get events -n "$(KIND_NAMESPACE)" --sort-by=.lastTimestamp || true; \
		echo "=== DCS LOGS (current) ==="; \
		"$(KUBECTL_BIN)" logs -n "$(KIND_NAMESPACE)" \
			-l app.kubernetes.io/component=backend \
			-c digital-contracting-service --tail=200 --prefix || true; \
		echo "=== DCS LOGS (previous crash) ==="; \
		"$(KUBECTL_BIN)" logs -n "$(KIND_NAMESPACE)" \
			-l app.kubernetes.io/component=backend \
			-c digital-contracting-service --tail=200 --prefix --previous || true; \
		echo "=== DCS POD DESCRIBE ==="; \
		"$(KUBECTL_BIN)" describe pod -n "$(KIND_NAMESPACE)" \
			-l app.kubernetes.io/component=backend || true; \
		echo "=== HSM PROVISION JOB LOGS ==="; \
		"$(KUBECTL_BIN)" logs -n "$(KIND_NAMESPACE)" \
			job/"$(DCS_DEPLOYMENT)"-hsm-provision --tail=100 || true; \
		exit 1; \
	fi

kind_uninstall:
	@"$(HELM_BIN)" uninstall "$(HELM_RELEASE)" --namespace "$(KIND_NAMESPACE)" >/dev/null 2>&1 || true

run_bdd_kind: kind_deploy
	@$(MAKE) run_bdd_helm \
		K8S_NAMESPACE="$(KIND_NAMESPACE)" \
		HELM_RELEASE="$(HELM_RELEASE)"

run_bdd_kind_all: kind_deploy
	@$(MAKE) run_bdd_helm_all \
		K8S_NAMESPACE="$(KIND_NAMESPACE)" \
		HELM_RELEASE="$(HELM_RELEASE)"

run_bdd_kind_ci: build_dcs_image build_pdf_core_image run_bdd_kind

clean:
	rm -rf "$(VENV_PATH)" .tmp .reports

# start kind cluster, deploy DCS and dependencies, and prepare for BDD runs
kind_up: build_dcs_image build_pdf_core_image kind_deploy setup_environment ensure_k8s_tools
	@echo "kind cluster $(KIND_CLUSTER_NAME) is up with DCS deployed in namespace $(KIND_NAMESPACE)"
	@echo "You can now run 'make run_bdd_kind_once' to execute the BDD suite against this cluster without redeploying between runs."

# run BDD suite once against kind deployment (without redeploying cluster between runs)
# Optionally pass F=<path> to run a single feature file, e.g. make run_bdd_kind_once F=features/login.feature
run_bdd_kind_once:
	@$(MAKE) run_bdd_helm \
		K8S_NAMESPACE="$(KIND_NAMESPACE)" \
		HELM_RELEASE="$(HELM_RELEASE)" \
		FEATURES_PATH="$(if $(F),$(F),$(FEATURES_PATH))"

# Shutdown deployment, cluster and clean test artifacts
kind_down: kind_uninstall kind_delete clean
