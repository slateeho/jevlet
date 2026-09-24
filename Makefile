SHELL := /usr/bin/env bash
IMAGE ?= ghcr.io/slateeho/jevlet:dev

.PHONY: fmt vet test build docker-build install uninstall
fmt:
	gofmt -w $$(find . -name '*.go' -type f)

vet:
	go vet ./...

test:
	go test ./...

build:
	go build ./cmd/manager

docker-build:
	docker build -t $(IMAGE) .

install:
	kubectl apply -k config/default

uninstall:
	kubectl delete -k config/default --ignore-not-found
