.PHONY: fmt fmt-check lint test build docker-build terraform-init terraform-validate check

BACKEND_DIR := app/backend
TERRAFORM_DIR := infra/terraform

fmt:
	cd $(BACKEND_DIR) && gofmt -w .
	cd $(TERRAFORM_DIR) && terraform fmt -recursive

fmt-check:
	cd $(BACKEND_DIR) && test -z "$$(gofmt -l .)"
	cd $(TERRAFORM_DIR) && terraform fmt -check -recursive -diff

lint:
	cd $(BACKEND_DIR) && go vet ./...
	cd $(BACKEND_DIR) && golangci-lint run --new-from-merge-base=main
test:
	cd $(BACKEND_DIR) && go test ./...

build:
	cd $(BACKEND_DIR) && go build ./...

docker-build:
	docker build -t sakura-c-backend:test ./app/backend

terraform-init:
	cd $(TERRAFORM_DIR) && terraform init -backend=false

terraform-validate: terraform-init
	cd $(TERRAFORM_DIR) && terraform validate

check: fmt-check lint test build docker-build terraform-validate
