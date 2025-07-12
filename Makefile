.PHONY: build run test clean docker-build docker-run docker-compose-up docker-compose-down

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
BINARY_NAME=frontend-router-mcp
BINARY_UNIX=$(BINARY_NAME)_unix
DOCKER_IMAGE=frontend-router-mcp

all: test build

build:
	$(GOBUILD) -o ./bin/$(BINARY_NAME) -v ./cmd/server

run: build
	./bin/$(BINARY_NAME)

test:
	$(GOTEST) -v ./...

clean:
	rm -f ./bin/$(BINARY_NAME)
	rm -f ./bin/$(BINARY_UNIX)

# Docker commands
docker-build:
	docker build -t $(DOCKER_IMAGE) .

docker-run: docker-build
	docker run -p 8080:8080 -p 8081:8081 $(DOCKER_IMAGE)

# Docker Compose commands
docker-compose-up:
	docker-compose up -d

docker-compose-down:
	docker-compose down

# Cross compilation
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o ./bin/$(BINARY_UNIX) -v ./cmd/server
