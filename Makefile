BINARY_NAME=pgadmin-cnpg-discovery
REGISTRY?=docker.io
DOCKER_USER?=ahmedmoalla
IMAGE_NAME=$(REGISTRY)/$(DOCKER_USER)/pgadmin-cnpg-discovery
IMAGE_TAG?=latest

.PHONY: build clean docker-build docker-push tidy test test-v test-coverage coverage-report lint

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/$(BINARY_NAME) ./cmd

clean:
	rm -rf bin/ coverage.out coverage.html

tidy:
	go mod tidy

test:
	go test ./...

test-v:
	go test ./... -v

test-coverage:
	go test ./... -cover -coverprofile=coverage.out

coverage-report: test-coverage
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

lint:
	go vet ./...

docker-build:
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .

docker-push: docker-build
	docker push $(IMAGE_NAME):$(IMAGE_TAG)
