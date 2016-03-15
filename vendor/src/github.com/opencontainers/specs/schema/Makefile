
default: help

help:
	@echo "Usage: make <target>"
	@echo
	@echo " * 'fmt' - format the json with indentation"
	@echo " * 'validate' - build the validation tool"

fmt:
	for i in *.json ; do jq --indent 4 -M . "$${i}" > xx && cat xx > "$${i}" && rm xx ; done

validate: validate.go
	go build ./validate.go

