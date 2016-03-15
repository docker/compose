
DOCKER ?= $(shell which docker)
DOC_FILES := \
	README.md \
	code-of-conduct.md \
	principles.md \
	style.md \
	ROADMAP.md \
	implementations.md \
	bundle.md \
	runtime.md \
	runtime-linux.md \
	config.md \
	config-linux.md \
	glossary.md
EPOCH_TEST_COMMIT := 041eb73d2e0391463894c04c8ac938036143eba3

docs: pdf html
.PHONY: docs

pdf:
	@mkdir -p output/ && \
	$(DOCKER) run \
	-it \
	--rm \
	-v $(shell pwd)/:/input/:ro \
	-v $(shell pwd)/output/:/output/ \
	-u $(shell id -u) \
	vbatts/pandoc -f markdown_github -t latex -o /output/docs.pdf $(patsubst %,/input/%,$(DOC_FILES)) && \
	ls -sh $(shell readlink -f output/docs.pdf)

html:
	@mkdir -p output/ && \
	$(DOCKER) run \
	-it \
	--rm \
	-v $(shell pwd)/:/input/:ro \
	-v $(shell pwd)/output/:/output/ \
	-u $(shell id -u) \
	vbatts/pandoc -f markdown_github -t html5 -o /output/docs.html $(patsubst %,/input/%,$(DOC_FILES)) && \
	ls -sh $(shell readlink -f output/docs.html)

.PHONY: test .govet .golint .gitvalidation

test: .govet .golint .gitvalidation

# `go get golang.org/x/tools/cmd/vet`
.govet:
	go vet -x ./...

# `go get github.com/golang/lint/golint`
.golint:
	golint ./...

# `go get github.com/vbatts/git-validation`
.gitvalidation:
	git-validation -q -run DCO,short-subject -v -range $(EPOCH_TEST_COMMIT)..HEAD

clean:
	rm -rf output/ *~

