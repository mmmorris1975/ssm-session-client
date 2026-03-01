# Acceptance test lifecycle Makefile.
# Usage: make acceptance AWS_REGION=ap-southeast-2

AWS_REGION     ?= $(shell aws configure get region 2>/dev/null || echo "ap-southeast-2")
INFRA_DIR      := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))../infra
TEST_DIR       := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))/..
OUTPUTS_JSON   := $(INFRA_DIR)/outputs.json
SSM_WAIT_SECS  ?= 300
SSM_POLL_SECS  ?= 10
TF_CMD         ?= $(shell command -v tofu 2>/dev/null || echo "terraform")
TF_EXTRA_VARS  ?=

# S3 backend for Terraform state.
# TF_STATE_BUCKET can be overridden by setting it in the environment (CI does
# this via a "Set Terraform state bucket" step after OIDC auth).  Locally it
# is computed from the current AWS caller identity.
TF_STATE_BUCKET ?= ssm-session-client-tfstate-$(shell aws sts get-caller-identity --query Account --output text 2>/dev/null)-$(AWS_REGION)
TF_BACKEND_FLAGS = \
	-backend-config="bucket=$(TF_STATE_BUCKET)" \
	-backend-config="key=acceptance/terraform.tfstate" \
	-backend-config="region=$(AWS_REGION)" \
	-backend-config="encrypt=true"

SSO_PROFILE        ?=
SSO_TEST_TIMEOUT   ?= 10

.PHONY: acceptance-prepare acceptance-wait-ssm acceptance-run-local acceptance-run-ci acceptance-destroy acceptance acceptance-sso-interactive help

help: ## Show this help message.
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-28s\033[0m %s\n", $$1, $$2}'

acceptance-prepare: ## Provision test infrastructure (requires AWS credentials).
	cd $(INFRA_DIR) && \
		$(TF_CMD) init -upgrade $(TF_BACKEND_FLAGS) && \
		$(TF_CMD) apply -auto-approve -var="region=$(AWS_REGION)" $(TF_EXTRA_VARS)

acceptance-wait-ssm: ## Wait for SSM agent to come online on provisioned instances.
	@echo "Waiting for SSM agent to become ready (timeout: $(SSM_WAIT_SECS)s)..."
	@INSTANCE_ID=$$(jq -r '.instance_id' $(OUTPUTS_JSON)); \
	WINDOWS_ID=$$(jq -r '.windows_instance_id // empty' $(OUTPUTS_JSON)); \
	DEADLINE=$$(($(shell date +%s) + $(SSM_WAIT_SECS))); \
	for ID in $$INSTANCE_ID $$WINDOWS_ID; do \
		[ -z "$$ID" ] && continue; \
		echo "  Checking instance $$ID ..."; \
		while true; do \
			STATUS=$$(aws ssm describe-instance-information \
				--region $(AWS_REGION) \
				--filters "Key=InstanceIds,Values=$$ID" \
				--query 'InstanceInformationList[0].PingStatus' \
				--output text 2>/dev/null); \
			if [ "$$STATUS" = "Online" ]; then \
				echo "  ✓ $$ID SSM agent is Online"; \
				break; \
			fi; \
			if [ $$(date +%s) -ge $$DEADLINE ]; then \
				echo "  ✗ Timed out waiting for SSM agent on $$ID" >&2; \
				exit 1; \
			fi; \
			echo "  … $$ID status=$$STATUS, retrying in $(SSM_POLL_SECS)s"; \
			sleep $(SSM_POLL_SECS); \
		done; \
	done
	@echo "All instances SSM-ready."

acceptance-run-local: ## Run acceptance tests locally (requires infra to be provisioned).
	cd $(TEST_DIR)/.. && \
		go test ./test/acceptance/... \
			-tags acceptance \
			-v \
			-timeout 20m \
			-count=1 \
			-race

acceptance-run-ci: ## Run acceptance tests in CI (identical to local; relies on OIDC credentials).
	$(MAKE) acceptance-run-local

acceptance-sso-interactive: ## Run interactive SSO login test (requires a human to approve in a browser).
	@if [ -z "$(SSO_PROFILE)" ]; then \
		echo "ERROR: SSO_PROFILE is required.  Usage: make acceptance-sso-interactive SSO_PROFILE=my-sso-profile" >&2; \
		exit 1; \
	fi
	cd $(TEST_DIR)/.. && \
		SSC_SSO_TEST_PROFILE=$(SSO_PROFILE) \
		SSC_SSO_TEST_INTERACTIVE=1 \
		SSC_SSO_TEST_TIMEOUT=$(SSO_TEST_TIMEOUT) \
		go test ./test/acceptance/... \
			-tags acceptance \
			-v \
			-timeout $$(( $(SSO_TEST_TIMEOUT) + 5 ))m \
			-count=1 \
			-run TestSSOLoginInteractive

acceptance-destroy: ## Tear down all test infrastructure (always runs even on failure).
	cd $(INFRA_DIR) && \
		$(TF_CMD) init -upgrade $(TF_BACKEND_FLAGS) && \
		$(TF_CMD) destroy -auto-approve -var="region=$(AWS_REGION)" $(TF_EXTRA_VARS)

acceptance: ## Full lifecycle: provision → wait → test → destroy (destroy runs even on test failure).
	$(MAKE) acceptance-prepare
	$(MAKE) acceptance-wait-ssm
	$(MAKE) acceptance-run-local; RESULT=$$?; \
	$(MAKE) acceptance-destroy; \
	exit $$RESULT
