
THRIFT_VER=0.9.2
THRIFT_IMG=thrift:$(THRIFT_VER)
THRIFT=docker run -u $(shell id -u) -v "${PWD}:/data" $(THRIFT_IMG) thrift

SWAGGER_VER=0.12.0
SWAGGER_IMAGE=quay.io/goswagger/swagger:$(SWAGGER_VER)
SWAGGER=docker run --rm -it -u ${shell id -u} -v "${PWD}:/go/src/${PROJECT_ROOT}" -w /go/src/${PROJECT_ROOT} $(SWAGGER_IMAGE)

PROTOTOOL_VER=1.8.0
PROTOTOOL_IMAGE=uber/prototool:$(PROTOTOOL_VER)
PROTOTOOL=docker run --rm -it -u ${shell id -u} -v "${PWD}:/go/src/${PROJECT_ROOT}" -w /go/src/${PROJECT_ROOT} $(PROTOTOOL_IMAGE)

THRIFT_GO_ARGS=thrift_import="github.com/apache/thrift/lib/go/thrift"
THRIFT_PY_ARGS=new_style,tornado
THRIFT_JAVA_ARGS=private-members
THRIFT_PHP_ARGS=psr4

THRIFT_GEN=--gen lua --gen go:$(THRIFT_GO_ARGS) --gen py:$(THRIFT_PY_ARGS) --gen java:$(THRIFT_JAVA_ARGS) --gen js:node --gen cpp --gen php:$(THRIFT_PHP_ARGS)
THRIFT_CMD=$(THRIFT) -o /data $(THRIFT_GEN)

THRIFT_FILES=agent.thrift jaeger.thrift sampling.thrift zipkincore.thrift crossdock/tracetest.thrift \
	baggage.thrift dependency.thrift aggregation_validator.thrift

test-ci: thrift swagger-validate protocompile proto

swagger-validate:
	$(SWAGGER) validate ./swagger/zipkin2-api.yaml

clean:
	rm -rf gen-* || true

thrift:	thrift-image clean $(THRIFT_FILES)

$(THRIFT_FILES):
	@echo Compiling $@
	$(THRIFT_CMD) /data/thrift/$@

thrift-image:
	docker pull $(THRIFT_IMG)
	$(THRIFT) -version

protocompile:
	$(PROTOTOOL) prototool compile proto --dry-run

JAEGER_DOCKER_PROTOBUF ?= jaegertracing/protobuf:0.1.0
PROTOC := docker run --rm -u ${shell id -u} -v${PWD}:${PWD} -w${PWD} ${JAEGER_DOCKER_PROTOBUF} --proto_path=${PWD}
PROTO_INCLUDES := \
	-Iproto \
	-I/usr/include/github.com/gogo/protobuf
# Remapping of std types to gogo types (must not contain spaces)
PROTO_GOGO_MAPPINGS := $(shell echo \
		Mgoogle/protobuf/descriptor.proto=github.com/gogo/protobuf/types, \
		Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types, \
		Mgoogle/protobuf/duration.proto=github.com/gogo/protobuf/types, \
		Mgoogle/protobuf/empty.proto=github.com/gogo/protobuf/types, \
		Mgoogle/api/annotations.proto=github.com/gogo/googleapis/google/api, \
		Mmodel.proto=github.com/jaegertracing/jaeger/model \
	| sed 's/ //g')

.PHONY: proto
proto:
	mkdir -p proto-gen
	$(PROTOC) \
		$(PROTO_INCLUDES) \
		--gogo_out=plugins=grpc,$(PROTO_GOGO_MAPPINGS):$(PWD)/proto-gen \
		proto/model.proto

	$(PROTOC) \
		$(PROTO_INCLUDES) \
		--gogo_out=plugins=grpc,$(PROTO_GOGO_MAPPINGS):$(PWD)/proto-gen \
		proto/api_v2/*.proto

	$(PROTOC) \
		$(PROTO_INCLUDES) \
		--gogo_out=plugins=grpc,$(PROTO_GOGO_MAPPINGS):$(PWD)/proto-gen \
		proto/zipkin.proto

.PHONY: test-ci clean thrift thrift-image $(THRIFT_FILES) swagger-validate protocompile proto
