# Copyright (c) 2023 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

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

# All .go files that are not auto-generated and should be auto-formatted and linted.
ALL_SRC = $(shell find . -name '*.go' \
				   -not -name '_*' \
				   -not -name '.*' \
				   -not -name 'mocks*' \
				   -not -name '*.pb.go' \
				   -not -path '*/gen-*/*' \
				   -not -path '*/thrift-0.9.2/*' \
				   -type f | \
				sort)

# All .sh or .py or Makefile or .mk files that should be auto-formatted and linted.
SCRIPTS_SRC = $(shell find . \( -name '*.sh' -o -name '*.py' -o -name '*.mk' -o -name 'Makefile*' -o -name 'Dockerfile*' \) \
						-not -path './.git/*' \
						-not -path '*/gen-*/*' \
						-not -path '*/scripts/*' \
						-not -name '_*' \
						-type f | \
					sort)

FMT_LOG=.fmt.log
IMPORT_LOG=.import.log

.PHONY: test
test: 
	echo $(SCRIPTS_SRC)

.PHONY: test-code-gen
test-code-gen: thrift swagger-validate protocompile proto proto-zipkin
	git diff --exit-code ./swagger/api_v3/query_service.swagger.json

.PHONY: swagger-validate
swagger-validate:
	$(SWAGGER) validate ./swagger/zipkin2-api.yaml

.PHONY: clean
clean:
	rm -rf *gen-* || true
	rm -rf .*gen-* || true

.PHONY: thrift
thrift:	thrift-image clean $(THRIFT_FILES)

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
lint: lint-imports lint-nocommit lint-license

.PHONY: lint-license
lint-license:
	@echo Verifying that all files have license headers
	@./scripts/lint/updateLicense.py $(ALL_SRC) $(SCRIPTS_SRC) > $(FMT_LOG)

.PHONY: lint-nocommit 
lint-nocommit:
	@if git diff origin/main | grep '@no''commit' ; then \
		echo "❌ Cannot merge PR that contains @no""commit string" ; \
		false ; \
	fi

.PHONY: lint-imports
lint-imports:
	@echo Verifying that all files have correctly ordered imports
	@./scripts/lint/import-order-cleanup.py -o stdout -t $(ALL_SRC) > $(IMPORT_LOG)
	@[ ! -s "$(IMPORT_LOG)" ] || (echo "Import ordering failures, run 'make fmt'" | cat - $(IMPORT_LOG) && false)

.PHONY: fmt
fmt: $(GOFUMPT)
	@echo Running import-order-cleanup on ALL_SRC ...
	@./scripts/lint/import-order-cleanup.py -o inplace -t $(ALL_SRC)
	@echo Running gofmt on ALL_SRC ...
	@$(GOFMT) -e -s -l -w $(ALL_SRC)
	@echo Running gofumpt on ALL_SRC ...
	@$(GOFUMPT) -e -l -w $(ALL_SRC)
	@echo Running updateLicense.py on ALL_SRC ...
	@./scripts/lint/updateLicense.py $(ALL_SRC) $(SCRIPTS_SRC)

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
	$(PROTOC) -Imodel/proto --go_out=$(PWD)/model/v1/ model/v1/prototest/model_test.proto

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
