.PHONY: all build install install-crds test clean ko-apply ko-publish

BIN_DIR := bin
PROJECT_ID ?= $(shell gcloud config get-value project 2>/dev/null || echo $$GOOGLE_CLOUD_PROJECT)
KO_DOCKER_REPO ?= gcr.io/$(PROJECT_ID)/geminiset
export KO_DOCKER_REPO

all: build test

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/ ./cmd/...

install:
	go install ./cmd/...

install-crds:
	kubectl apply -f deploy/crds/geminiset.io_geminisets.yaml

test:
	go test -v ./...

ko-apply:
	ko apply -f deploy/operator.yaml

ko-publish:
	ko build ./cmd/gemini-operator --bare

clean:
	rm -rf $(BIN_DIR)
