#
#   ConsensusDB docker maker
#
#   Alex Shvid
#
#

IMAGE := consensusdb
VERSION := $(shell git describe --tags --always --dirty)
TAG := $(VERSION)
REGISTRY := shvid
PWD := $(shell pwd)

all: run

version:
	@echo $(TAG)

build: version
	go test -cover ./...
	go build

run: build
	env COS=dev ./consensusdb

test: build
	env COS=test ./consensusdb

docker:
	docker build  --build-arg TAG=$(TAG) -t $(REGISTRY)/$(IMAGE):$(TAG) -f Dockerfile .

docker-run: docker
	docker run -p 3360:3360 -p 3361:3361 --env COS -v $(PWD)/config:/app/config $(REGISTRY)/$(IMAGE):$(TAG)

docker-push: docker
	docker push ${REGISTRY}/${IMAGE}:${TAG}
	docker tag ${REGISTRY}/${IMAGE}:${TAG} ${REGISTRY}/${IMAGE}:latest
	docker push ${REGISTRY}/${IMAGE}:latest

