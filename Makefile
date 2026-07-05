#
#   ConsensusDB
#
#   Copyright (c) 2025 Karagatan LLC.
#   SPDX-License-Identifier: BUSL-1.1
#

IMAGE    := consensusdb
VERSION  := $(shell git describe --tags --always --dirty)
TAG      := $(VERSION)
REGISTRY := arpabet
PWD      := $(shell pwd)
NOW      := $(shell date +"%m-%d-%Y")

# Full build, like gazile's `make all`: build + embed the admin console, then
# build the binary (which runs vet + tests first). Needs Node for `webui`.
all: webui build

version:
	@echo $(TAG)

# Install the asset bundler used by `make webui`.
deps:
	go install go.arpabet.com/go-bindata/go-bindata@v1.1.0

# Rebuild the embedded web apps: build the Vite project (webapp/) and bake
# webapp/dist into pkg/webui via go-bindata. pkg/webui/bindata.go is generated
# (git-ignored), so run this (or `make all`) before `go build`; the release and
# Docker builds regenerate it themselves. A fresh checkout has an empty pkg/webui
# until this runs.
webui:
	npm --prefix webapp ci
	npm --prefix webapp run build
	go-bindata -pkg webui -o pkg/webui/bindata.go -fs -nocompress -nomemcopy -prefix "webapp/dist/" webapp/dist/...

clean:
	go clean -i ./...

vet:
	go vet ./...

test: vet
	go test -race -cover ./...

build: test
	go build -v -ldflags "-X main.Version=$(VERSION) -X main.Built=$(NOW)"

run: build
	env COS=dev ./consensusdb run

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

update:
	go get -u ./...

licenses:
	go run github.com/google/go-licenses@latest csv "go.arpabet.com/consensusdb" > pkg/res/licenses.txt

docker:
	docker build --build-arg TAG=$(TAG) -t $(REGISTRY)/$(IMAGE):$(TAG) -f Dockerfile .

docker-run: docker
	docker run -p 3360:3360 -p 3361:3361 --env COS -v $(PWD)/config:/app/config $(REGISTRY)/$(IMAGE):$(TAG)

docker-push: docker
	docker push ${REGISTRY}/${IMAGE}:${TAG}
	docker tag ${REGISTRY}/${IMAGE}:${TAG} ${REGISTRY}/${IMAGE}:latest
	docker push ${REGISTRY}/${IMAGE}:latest

.PHONY: all version deps webui clean vet test build run vuln update licenses docker docker-run docker-push
