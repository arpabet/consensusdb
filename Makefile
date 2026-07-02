#
#   ConsensusDB
#
#   Copyright (c) 2025 Karagatan LLC.
#   SPDX-License-Identifier: Apache-2.0
#

IMAGE    := consensusdb
VERSION  := $(shell git describe --tags --always --dirty)
TAG      := $(VERSION)
REGISTRY := arpabet
PWD      := $(shell pwd)
NOW      := $(shell date +"%m-%d-%Y")

all: build

version:
	@echo $(TAG)

clean:
	go clean -i ./...

vet:
	go vet ./...

test: vet
	go test -race -cover ./...

build: test
	go build -v -ldflags "-X main.Version=$(VERSION) -X main.Built=$(NOW)"

run: build
	env COS=dev ./consensusdb

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

update:
	go get -u ./...

licenses:
	go run github.com/google/go-licenses@latest csv "go.arpabet.com/consensusdb" > pkg/res/licenses.txt

genproto:
	./genproto.sh

docker:
	docker build --build-arg TAG=$(TAG) -t $(REGISTRY)/$(IMAGE):$(TAG) -f Dockerfile .

docker-run: docker
	docker run -p 3360:3360 -p 3361:3361 --env COS -v $(PWD)/config:/app/config $(REGISTRY)/$(IMAGE):$(TAG)

docker-push: docker
	docker push ${REGISTRY}/${IMAGE}:${TAG}
	docker tag ${REGISTRY}/${IMAGE}:${TAG} ${REGISTRY}/${IMAGE}:latest
	docker push ${REGISTRY}/${IMAGE}:latest

.PHONY: all version clean vet test build run vuln update licenses genproto docker docker-run docker-push
