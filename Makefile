.PHONY: build test test-short test-integration test-e2e vet vulncheck vendor docker

build:
	go build -mod=vendor -trimpath -o mcp-k8s-ephemeral-job ./cmd/mcp-k8s-ephemeral-job

test:
	go test ./...

test-short:
	go test -short ./...

test-integration:
	go test -tags envtest ./tests/integration/...

test-e2e:
	go test -tags kind ./tests/integration/...

vet:
	go vet ./...

vulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

vendor:
	go mod tidy && go mod vendor

docker:
	docker build -t idconstruct/mcp-k8s-ephemeral-job:dev .
