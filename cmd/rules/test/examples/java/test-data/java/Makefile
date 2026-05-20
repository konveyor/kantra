# These can be overidden with environment variables of the same name
REGISTRY ?= quay.io
REPOSITORY ?= rofrano
IMAGE ?= customers-tomcat
TAG ?= 0.1
LOCAL_IMAGE_NAME ?= $(IMAGE):$(TAG)
REMOTE_IMAGE_NAME ?= $(REGISTRY)/$(REPOSITORY)/$(IMAGE):$(TAG)

all: build

## help:	Lists help on the commands
.PHONY: help
help: Makefile
	@sed -ne '/@sed/!s/## //p' $(MAKEFILE_LIST)

## Removes all dangling and built images
.PHONY: clean
clean: remove
	$(info Removing all dangling build cache)
	echo Y | docker image prune

.PHONY: build
build:	## Build all of the project Docker images
	$(info Building $(LOCAL_IMAGE_NAME) image...)
	docker build -t $(LOCAL_IMAGE_NAME) .

.PHONY: run
run:	## Run a vagrant VM using this image
	$(info Bringing up $(LOCAL_IMAGE_NAME)...)
	docker run --rm -p 8080:8080 $(LOCAL_IMAGE_NAME)

.PHONY: remove
remove:	## Removes all built images
	$(info Removing all built images...)
	docker rmi $(REMOTE_IMAGE_NAME)
	docker rmi $(LOCAL_IMAGE_NAME)

.PHONY: push
push: ## Push image to docker repository
	$(info Pushing $(LOCAL_IMAGE_NAME) image...)
	docker tag $(LOCAL_IMAGE_NAME) $(REMOTE_IMAGE_NAME)
	docker push $(REMOTE_IMAGE_NAME)
