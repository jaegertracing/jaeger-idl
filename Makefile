# Copyright (c) 2023 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0
SCRIPTS_URL = https://raw.githubusercontent.com/jaegertracing/jaeger/main/scripts

JAEGER_IMPORT_PATH = github.com/jaegertracing/jaeger-idl

THRIFT_VER?=0.19
THRIFT_IMG?=jaegertracing/thrift:$(THRIFT_VER)
THRIFT=docker run --rm -u $(shell id -u) -v "${PWD}:/data" $(THRIFT_IMG) thrift

SWAGGER_VER=0.31.0
SWAGGER_IMAGE=quay.io/goswagger/swagger:$(SWAGGER_VER)
SWAGGER=docker run --rm -u ${shell id -u} -v "${PWD}:/go/src/${PROJECT_ROOT}" -w /go/src/${PROJECT_ROOT} $(SWAGGER_IMAGE)

PROTOTOOL_VER=1.8.0
PROTOTOOL_IMAGE=uber/prototool:$(PROTOTOOL_VER)
PROTOTOOL=docker run --rm -u ${shell id -u} -v "${PWD}:/go/src/${PROJECT_ROOT}" -w /go/src/${PROJECT_ROOT} $(PROTOTOOL_IMAGE)

PROTOC_VER=0.5.0
PROTOC_IMAGE=jaegertracing/protobuf:$(PROTOC_VER)
PROTOC=docker run --rm -u ${shell id -u} -v "${PWD}:${PWD}" -w ${PWD} ${PROTOC_IMAGE} --proto_path=${PWD}

THRIFT_GO_ARGS=thrift_import="github.com/apache/thrift/lib/go/thrift"
THRIFT_PY_ARGS=new_style,tornado
THRIFT_JAVA_ARGS=private-members
THRIFT_PHP_ARGS=psr4

THRIFT_GEN=--gen lua --gen go:$(THRIFT_GO_ARGS) --gen py:$(THRIFT_PY_ARGS) --gen java:$(THRIFT_JAVA_ARGS) --gen js:node --gen cpp --gen php:$(THRIFT_PHP_ARGS)
THRIFT_CMD=$(THRIFT) -o /data $(THRIFT_GEN)

THRIFT_FILES=agent.thrift jaeger.thrift sampling.thrift zipkincore.thrift crossdock/tracetest.thrift \
	baggage.thrift dependency.thrift aggregation_validator.thrift
THRIFT_GEN_DIR=thrift-gen

# All .go files that are not auto-generated and should be auto-formatted and linted.
ALL_SRC = $(shell find . -name '*.go' \
				   -not -name '_*' \
				   -not -name '.*' \
				   -not -name '*.pb.go' \
				   -not -path './thrift-gen/*'\
				   -type f | \
				sort)


FMT_LOG=.fmt.log
IMPORT_LOG=.import.log

# SRC_ROOT is the top of the source tree.
SRC_ROOT := $(shell git rev-parse --show-toplevel)
TOOLS_MOD_DIR   := $(SRC_ROOT)/internal/tools
TOOLS_BIN_DIR   := $(SRC_ROOT)/.tools
LINT         := $(TOOLS_BIN_DIR)/golangci-lint

$(TOOLS_BIN_DIR):
	mkdir -p $@

$(LINT): $(TOOLS_BIN_DIR)
	cd $(TOOLS_MOD_DIR) && go build -o $@ github.com/golangci/golangci-lint/cmd/golangci-lint

.PHONY: test-code-gen
test-code-gen: thrift-all swagger-validate protocompile proto-all proto-zipkin
	git diff --exit-code ./swagger/api_v3/query_service.swagger.json

.PHONY: swagger-validate
swagger-validate:
	$(SWAGGER) validate ./swagger/zipkin2-api.yaml

.PHONY: clean
clean:
	rm -rf *gen-* || true
	rm -rf .*gen-* || true
	rm -rf coverage.txt

.PHONY: thrift
thrift:
	[ -d $(THRIFT_GEN_DIR) ] || mkdir $(THRIFT_GEN_DIR)
	$(THRIFT) -o /data --gen go:$(THRIFT_GO_ARGS) -out /data/$(THRIFT_GEN_DIR) /data/thrift/jaeger.thrift
	$(THRIFT) -o /data --gen go:$(THRIFT_GO_ARGS) -out /data/$(THRIFT_GEN_DIR) /data/thrift/zipkincore.thrift
	$(THRIFT) -o /data --gen go:$(THRIFT_GO_ARGS) -out /data/$(THRIFT_GEN_DIR) /data/thrift/agent.thrift
	$(THRIFT) -o /data --gen go:$(THRIFT_GO_ARGS) --out /data/$(THRIFT_GEN_DIR) /data/thrift/sampling.thrift
	$(SED) -i.bak 's|"zipkincore"|"$(JAEGER_IMPORT_PATH)/thrift-gen/zipkincore"|g' $(THRIFT_GEN_DIR)/agent/*.go
	$(SED) -i.bak 's|"jaeger"|"$(JAEGER_IMPORT_PATH)/thrift-gen/jaeger"|g' $(THRIFT_GEN_DIR)/agent/*.go
	rm -rf thrift-gen/*/*-remote thrift-gen/*/*.bak

.PHONY: thrift-all
thrift-all: thrift-image clean $(THRIFT_FILES)

$(THRIFT_FILES):
	@echo Compiling $@
	$(THRIFT_CMD) /data/thrift/$@

.PHONY: thrift-image
thrift-image:
	docker pull $(THRIFT_IMG)
	$(THRIFT) -version

.PHONY: protocompile
protocompile:
	$(PROTOTOOL) prototool compile proto --dry-run

PROTO_INCLUDES := \
	-Iproto/api_v2 \
	-Iproto \
	-I/usr/include/github.com/gogo/protobuf \
	-Iopentelemetry-proto
# Remapping of std types to gogo types (must not contain spaces)
PROTO_GOGO_MAPPINGS := $(shell echo \
		Mgoogle/protobuf/descriptor.proto=github.com/gogo/protobuf/types, \
		Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types, \
		Mgoogle/protobuf/duration.proto=github.com/gogo/protobuf/types, \
		Mgoogle/protobuf/empty.proto=github.com/gogo/protobuf/types, \
		Mgoogle/api/annotations.proto=github.com/gogo/googleapis/google/api, \
		Mmodel.proto=github.com/jaegertracing/jaeger-idl/model/v1 \
	| sed 's/ //g')

PROTO_GEN_GO_DIR ?= proto-gen
POLYGLOT_DIR_ROOT ?= .proto-gen-polyglot
PROTO_GEN_GO_DIR_POLYGLOT ?= $(POLYGLOT_DIR_ROOT)/proto-gen-go
PROTO_GEN_PYTHON_DIR_POLYGLOT ?= $(POLYGLOT_DIR_ROOT)/proto-gen-python
PROTO_GEN_JAVA_DIR_POLYGLOT ?= $(POLYGLOT_DIR_ROOT)/proto-gen-java
PROTO_GEN_JS_DIR_POLYGLOT ?= $(POLYGLOT_DIR_ROOT)/proto-gen-js
PROTO_GEN_CPP_DIR_POLYGLOT ?= $(POLYGLOT_DIR_ROOT)/proto-gen-cpp
PROTO_GEN_CSHARP_DIR_POLYGLOT ?= $(POLYGLOT_DIR_ROOT)/proto-gen-csharp

API_V2_PATH ?= api_v2

# The jaegertracing/protobuf container image does not
# include Java/C#/C++ plugins for Apple Silicon (arm64).

PROTOC_WITHOUT_GRPC_common := $(PROTOC) \
		$(PROTO_INCLUDES) \
		--gogo_out=plugins=grpc,$(PROTO_GOGO_MAPPINGS):$(PWD)/${PROTO_GEN_GO_DIR_POLYGLOT} \
		--python_out=${PROTO_GEN_PYTHON_DIR_POLYGLOT} \
		--js_out=${PROTO_GEN_JS_DIR_POLYGLOT}

ifeq ($(shell uname -m),arm64)
PROTOC_WITHOUT_GRPC := $(PROTOC_WITHOUT_GRPC_common)
else
PROTOC_WITHOUT_GRPC := $(PROTOC_WITHOUT_GRPC_common) \
		--java_out=${PROTO_GEN_JAVA_DIR_POLYGLOT} \
		--cpp_out=${PROTO_GEN_CPP_DIR_POLYGLOT} \
		--csharp_out=base_namespace:${PROTO_GEN_CSHARP_DIR_POLYGLOT}
endif

PROTOC_WITH_GRPC_common := $(PROTOC_WITHOUT_GRPC) \
		--grpc-python_out=${PROTO_GEN_PYTHON_DIR_POLYGLOT} \
		--grpc-js_out=${PROTO_GEN_JS_DIR_POLYGLOT}

ifeq ($(shell uname -m),arm64)
PROTOC_WITH_GRPC := $(PROTOC_WITH_GRPC_common)
else
PROTOC_WITH_GRPC := $(PROTOC_WITH_GRPC_common) \
		--grpc-java_out=${PROTO_GEN_JAVA_DIR_POLYGLOT} \
		--grpc-cpp_out=${PROTO_GEN_CPP_DIR_POLYGLOT} \
		--grpc-csharp_out=${PROTO_GEN_CSHARP_DIR_POLYGLOT}
endif

PROTOC_INTERNAL := $(PROTOC) \
		$(PROTO_INCLUDES) \
		--csharp_out=internal_access,base_namespace:${PROTO_GEN_CSHARP_DIR_POLYGLOT} \
		--python_out=${PROTO_GEN_PYTHON_DIR_POLYGLOT}

GO=go
GOOS ?= $(shell $(GO) env GOOS)
GOARCH ?= $(shell $(GO) env GOARCH)

# sed on Mac does not support the same syntax for in-place updates as sed on Linux
# When running on MacOS it's best to install gsed and run Makefile with SED=gsed
ifeq ($(GOOS),darwin)
	SED=gsed
else
	SED=sed
endif

# DO NOT DELETE EMPTY LINE at the end of the macro, it's required to separate commands.
define print_caption
  @echo "🏗️ "
  @echo "🏗️ " $1
  @echo "🏗️ "

endef

# Macro to compile Protobuf $(2) into directory $(1). $(3) can provide additional flags.
# DO NOT DELETE EMPTY LINE at the end of the macro, it's required to separate commands.
# Arguments:
#  $(1) - output directory
#  $(2) - path to the .proto file
#  $(3) - additional flags to pass to protoc, e.g. extra -Ixxx
#  $(4) - additional options to pass to gogo plugin
define proto_compile
  $(call print_caption, "Processing $(2) --> $(1)")

  $(PROTOC) \
    $(PROTO_INCLUDES) \
    --gogo_out=plugins=grpc,$(strip $(4)),$(PROTO_GOGO_MAPPINGS):$(PWD)/$(strip $(1)) \
    $(3) $(2)

endef

.PHONY: lint
lint: lint-imports lint-nocommit lint-license lint-go

.PHONY: lint-go
lint-go: $(LINT)
	$(LINT) -v run

.PHONY: lint-license
lint-license: setup-lint-scripts
	@echo Verifying that all files have license headers
	@mkdir -p .scripts/lint
	@curl -s -o .scripts/lint/updateLicense.py https://raw.githubusercontent.com/jaegertracing/jaeger/main/scripts/lint/updateLicense.py
	@chmod +x .scripts/lint/updateLicense.py
	@./.scripts/lint/updateLicense.py $(ALL_SRC) $(SCRIPTS_SRC) > $(FMT_LOG)
	@[ -s "$(FMT_LOG)" ] || echo "✅ All files have license headers"


.PHONY: lint-nocommit 
lint-nocommit:
	@if git diff origin/main | grep '@no''commit' ; then \
		echo "❌ Cannot merge PR that contains @no""commit string" ; \
		false ; \
	fi

.PHONY: lint-imports
lint-imports: setup-lint-scripts
	@echo Verifying that all files have correctly ordered imports
	@./.scripts/lint/import-order-cleanup.py -o stdout -t $(ALL_SRC) > $(IMPORT_LOG)
	@[ ! -s "$(IMPORT_LOG)" ] || (echo "Import ordering failures, run 'make fmt'" | cat - $(IMPORT_LOG) && false)
	@[ -s "$(IMPORT_LOG)" ] || echo "✅ All files have correctly ordered imports"

.PHONY: setup-lint-scripts
setup-lint-scripts:
	@mkdir -p .scripts/lint
	@curl -s -o .scripts/lint/import-order-cleanup.py $(SCRIPTS_URL)/lint/import-order-cleanup.py
	@chmod +x .scripts/lint/import-order-cleanup.py
	@curl -s -o .scripts/lint/updateLicense.py $(SCRIPTS_URL)/lint/updateLicense.py
	@chmod +x .scripts/lint/updateLicense.py

.PHONY: fmt
fmt: setup-lint-scripts $(GOFUMPT)
	@echo Running import-order-cleanup on ALL_SRC ...
	@./.scripts/lint/import-order-cleanup.py -o inplace -t $(ALL_SRC)
	@echo Running gofmt on ALL_SRC ...
	@$(GOFMT) -e -s -l -w $(ALL_SRC)
	@echo Running gofumpt on ALL_SRC ...
	@$(GOFUMPT) -e -l -w $(ALL_SRC)
	@echo Running updateLicense.py on ALL_SRC ...
	@./.scripts/lint/updateLicense.py $(ALL_SRC) $(SCRIPTS_SRC)

.PHONY: test-ci
test-ci:
	go test -v -coverprofile=coverage.txt ./...

# proto target is used to generate source code that is released as part of this library
proto: proto-prepare proto-api-v2 proto-prototest

# proto-all target is used to generate code for all languages as a validation step.
proto-all: proto-prepare-all proto-api-v2-all proto-api-v3-all

.PHONY: proto-prepare-all
proto-prepare-all:
	mkdir -p ${PROTO_GEN_GO_DIR_POLYGLOT} \
		${PROTO_GEN_JAVA_DIR_POLYGLOT} \
		${PROTO_GEN_PYTHON_DIR_POLYGLOT} \
		${PROTO_GEN_JS_DIR_POLYGLOT} \
		${PROTO_GEN_CPP_DIR_POLYGLOT} \
		${PROTO_GEN_CSHARP_DIR_POLYGLOT}

.PHONY: proto-prepare
proto-prepare:
	mkdir -p ${PROTO_GEN_GO_DIR}

.PHONY: proto-prototest
proto-prototest:
	$(PROTOC) --go_out=$(PWD)/model/v1/ model/v1/prototest/model_test.proto

.PHONY: proto-api-v2
proto-api-v2:
	mkdir -p ${PROTO_GEN_GO_DIR}/${API_V2_PATH}
	$(call proto_compile, model/v1, proto/api_v2/model.proto)
	$(call proto_compile, ${PROTO_GEN_GO_DIR}/${API_V2_PATH}, proto/api_v2/query.proto)
	$(call proto_compile, ${PROTO_GEN_GO_DIR}/${API_V2_PATH}, proto/api_v2/collector.proto)
	$(call proto_compile, ${PROTO_GEN_GO_DIR}/${API_V2_PATH}, proto/api_v2/sampling.proto)

.PHONY: proto-api-v2-all
proto-api-v2-all:
	$(PROTOC_WITHOUT_GRPC) \
		proto/api_v2/model.proto

	$(PROTOC_WITH_GRPC) \
		proto/api_v2/query.proto \
		proto/api_v2/collector.proto \
		proto/api_v2/sampling.proto


.PHONY: proto-api-v3-all
proto-api-v3-all:
	# API v3
	$(PROTOC_WITH_GRPC) \
		proto/api_v3/query_service.proto
	# GRPC gateway
	$(PROTOC) \
		$(PROTO_INCLUDES) \
 		--grpc-gateway_out=logtostderr=true,grpc_api_configuration=proto/api_v3/query_service_http.yaml,$(PROTO_GOGO_MAPPINGS):${PROTO_GEN_GO_DIR_POLYGLOT} \
		proto/api_v3/query_service.proto
	# Swagger
	$(PROTOC) \
		$(PROTO_INCLUDES) \
		--swagger_out=disable_default_errors=true,logtostderr=true,grpc_api_configuration=proto/api_v3/query_service_http.yaml:./swagger \
		proto/api_v3/query_service.proto

	$(PROTOC_INTERNAL) \
		google/api/annotations.proto \
		google/api/http.proto \
		protoc-gen-swagger/options/annotations.proto \
		protoc-gen-swagger/options/openapiv2.proto \
		gogoproto/gogo.proto

.PHONY: proto-zipkin
proto-zipkin: proto-prepare-all
	$(PROTOC_WITHOUT_GRPC) \
		proto/zipkin.proto

.PHONY: init-submodule
init-submodule:
	git submodule init
	git submodule update --recursive
