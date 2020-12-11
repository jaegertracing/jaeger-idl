
THRIFT_VER?=0.13
THRIFT_IMG?=jaegertracing/thrift:$(THRIFT_VER)
THRIFT=docker run --rm -u $(shell id -u) -v "${PWD}:/data" $(THRIFT_IMG) thrift

SWAGGER_VER=0.12.0
SWAGGER_IMAGE=quay.io/goswagger/swagger:$(SWAGGER_VER)
SWAGGER=docker run --rm -u ${shell id -u} -v "${PWD}:/go/src/${PROJECT_ROOT}" -w /go/src/${PROJECT_ROOT} $(SWAGGER_IMAGE)

PROTOTOOL_VER=1.8.0
PROTOTOOL_IMAGE=uber/prototool:$(PROTOTOOL_VER)
PROTOTOOL=docker run --rm -u ${shell id -u} -v "${PWD}:/go/src/${PROJECT_ROOT}" -w /go/src/${PROJECT_ROOT} $(PROTOTOOL_IMAGE)

PROTOC_VER=0.2.0
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

test-ci: thrift swagger-validate protocompile proto proto-zipkin

swagger-validate:
	$(SWAGGER) validate ./swagger/zipkin2-api.yaml

clean:
	rm -rf *gen-* || true

thrift:	thrift-image clean $(THRIFT_FILES)

$(THRIFT_FILES):
	@echo Compiling $@
	$(THRIFT_CMD) /data/thrift/$@

thrift-image:
	docker pull $(THRIFT_IMG)
	$(THRIFT) -version

protocompile:
	$(PROTOTOOL) prototool compile proto --dry-run

PROTO_INCLUDES := \
	-Iproto/api_v2 \
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

PROTO_GEN_GO_DIR ?= proto-gen-go
PROTO_GEN_PYTHON_DIR ?= proto-gen-python
PROTO_GEN_JAVA_DIR ?= proto-gen-java
PROTO_GEN_JS_DIR ?= proto-gen-js
PROTO_GEN_CPP_DIR ?= proto-gen-cpp
PROTO_GEN_CSHARP_DIR ?= proto-gen-csharp

PROTOC_WITHOUT_GRPC := $(PROTOC) \
		$(PROTO_INCLUDES) \
		--gogo_out=plugins=grpc,$(PROTO_GOGO_MAPPINGS):$(PWD)/${PROTO_GEN_GO_DIR} \
		--java_out=${PROTO_GEN_JAVA_DIR} \
		--python_out=${PROTO_GEN_PYTHON_DIR} \
		--js_out=${PROTO_GEN_JS_DIR} \
		--cpp_out=${PROTO_GEN_CPP_DIR} \
		--csharp_out=base_namespace:${PROTO_GEN_CSHARP_DIR}

PROTOC_WITH_GRPC := $(PROTOC_WITHOUT_GRPC) \
		--grpc-java_out=${PROTO_GEN_JAVA_DIR} \
		--grpc-python_out=${PROTO_GEN_PYTHON_DIR} \
		--grpc-js_out=${PROTO_GEN_JS_DIR} \
		--grpc-cpp_out=${PROTO_GEN_CPP_DIR} \
		--grpc-csharp_out=${PROTO_GEN_CSHARP_DIR}

PROTOC_CS_INTERNAL := $(PROTOC) \
		$(PROTO_INCLUDES) \
		--csharp_out=internal_access,base_namespace:${PROTO_GEN_CSHARP_DIR}

 PROTOC_PY_INTERNAL := $(PROTOC) \
		$(PROTO_INCLUDES) \
		--python_out=${PROTO_GEN_PYTHON_DIR}

proto:
	mkdir -p ${PROTO_GEN_GO_DIR} \
		${PROTO_GEN_JAVA_DIR} \
		${PROTO_GEN_PYTHON_DIR} \
		${PROTO_GEN_JS_DIR} \
		${PROTO_GEN_CPP_DIR} \
		${PROTO_GEN_CSHARP_DIR}

	$(PROTOC_WITHOUT_GRPC) \
		proto/api_v2/model.proto
	
	$(PROTOC_WITH_GRPC) \
		proto/api_v2/query.proto \
		proto/api_v2/collector.proto \
		proto/api_v2/sampling.proto

	$(PROTOC_CS_INTERNAL) \
		google/api/annotations.proto \
		google/api/http.proto \
		protoc-gen-swagger/options/annotations.proto \
		protoc-gen-swagger/options/openapiv2.proto \
		gogoproto/gogo.proto

	$(PROTOC_PY_INTERNAL) \
		google/api/annotations.proto \
		google/api/http.proto \
		protoc-gen-swagger/options/annotations.proto \
		protoc-gen-swagger/options/openapiv2.proto \
			/gogo.proto

proto-zipkin:
	$(PROTOC_WITHOUT_GRPC) \
		proto/zipkin.proto

.PHONY: test-ci clean thrift thrift-image $(THRIFT_FILES) swagger-validate protocompile proto proto-zipkin
