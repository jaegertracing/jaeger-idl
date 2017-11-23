DOCKER_RUN=docker run -it --rm -v "${PWD}:/data:rw" -u ${shell id -u}:${shell id -g} -w /data

THRIFT_VER=0.9.2
THRIFT_IMG=thrift:$(THRIFT_VER)
THRIFT=docker run -u $(shell id -u) -v "${PWD}:/data" $(THRIFT_IMG) thrift

SWAGGER_VER=0.12.0
SWAGGER_IMAGE=quay.io/goswagger/swagger:$(SWAGGER_VER)
SWAGGER=docker run --rm -it -u ${shell id -u} -v "${PWD}:/go/src/${PROJECT_ROOT}" -w /go/src/${PROJECT_ROOT} $(SWAGGER_IMAGE)

THRIFT_GO_ARGS=thrift_import="github.com/apache/thrift/lib/go/thrift"
THRIFT_PY_ARGS=new_style,tornado
THRIFT_JAVA_ARGS=private-members
THRIFT_PHP_ARGS=psr4

THRIFT_GEN=--gen go:$(THRIFT_GO_ARGS) --gen py:$(THRIFT_PY_ARGS) --gen java:$(THRIFT_JAVA_ARGS) --gen js:node --gen cpp --gen php:$(THRIFT_PHP_ARGS)
THRIFT_CMD=$(THRIFT) -o /data $(THRIFT_GEN)

THRIFT_FILES=agent.thrift jaeger.thrift sampling.thrift zipkincore.thrift crossdock/tracetest.thrift baggage.thrift dependency.thrift aggregation_validator.thrift

PROTOC_VER=0.1
PROTOC_IMG=znly/protoc:$(PROTOC_VER)
PROTOC=$(DOCKER_RUN) $(PROTOC_IMG)
PROTOC_OUT=--gofast_out=/data/pb-go --java_out=/data/pb-java \
		   --js_out=/data/pb-js --python_out=/data/pb-py --cpp_out=/data/pb-cpp
PROTOBUF_DIRS=pb-go pb-java pb-js pb-py pb-cpp
PROTOBUF_FILES=agent.proto baggage.proto jaeger.proto sampling.proto

test-ci: thrift swagger-validate protobuf

swagger-validate:
	$(SWAGGER) validate ./swagger/zipkin2-api.yaml

clean: thrift-clean protobuf-clean


thrift-clean:
	rm -rf gen-* || true

thrift:	thrift-image thrift-clean $(THRIFT_FILES)

$(THRIFT_FILES):
	@echo Compiling $@
	$(THRIFT_CMD) /data/thrift/$@

thrift-image:
	docker pull $(THRIFT_IMG)
	$(THRIFT) -version

protobuf-clean:
	rm -rf pb-* || true

$(PROTOBUF_DIRS):
	mkdir $@

protobuf: protobuf-image protobuf-clean $(PROTOBUF_FILES)

protobuf-image:
	docker pull $(PROTOC_IMG)
	$(PROTOC) --version

$(PROTOBUF_FILES): $(PROTOBUF_DIRS)
	@echo "Compiling $@"
	$(PROTOC) $(PROTOC_OUT) -I/data/protobuf /data/protobuf/$@

.PHONY: test-ci thrift-clean protobuf-clean clean \
	thrift thrift-image $(THRIFT_FILES) swagger-validate \
	protobuf protobuf-image $(PROTOBUF_FILES)
