default: help

.PHONY: help
help: ## list makefile targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

PHONY: fmt
fmt: ## format go files
	gofumpt -w .
	gci write .

PHONY: lint
lint: ## lint go files
	golangci-lint run -c .golang-ci.yml

.PHONY: vault
vault: ## run vault server
	kill $(pgrep -x vault) || true
	vault server -dev -dev-root-token-id=root

.PHONY: vault-load-gen
vault-load-gen: ## run vault-load-gen
	docker run \
  		--name=vault-benchmark \
		--rm \
  		--hostname=vault-benchmark \
  		-v ./scripts/load-gen:/opt/vault-benchmark/configs \
  		hashicorp/vault-benchmark:latest \
  		vault-benchmark run -config=/opt/vault-benchmark/configs/config.hcl

.PHONY: docker
docker: ## build docker image
	cd docker && docker-compose up
