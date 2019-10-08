# Variables for Fossa
BUILD_ANALYZER?=docker/fossa-analyzer
FOSSA_OPTS?=--option all-tags:true --option allow-unresolved:true

fossa-analyze:
	docker run --rm -e FOSSA_API_KEY=$(FOSSA_API_KEY) \
		-v $(CURDIR)/$*:/go/src/github.com/docker/compose \
		-w /go/src/github.com/docker/compose \
		$(BUILD_ANALYZER) analyze ${FOSSA_OPTS} --branch ${BRANCH_NAME}

 # This command is used to run the fossa test command
fossa-test:
	docker run -i -e FOSSA_API_KEY=$(FOSSA_API_KEY) \
		-v $(CURDIR)/$*:/go/src/github.com/docker/compose \
		-w /go/src/github.com/docker/compose \
		$(BUILD_ANALYZER) test
