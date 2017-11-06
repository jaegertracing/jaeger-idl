
THRIFT_VER=0.10.0
THRIFT_IMG=thrift:$(THRIFT_VER)
THRIFT=docker run -v "${PWD}:/data" $(THRIFT_IMG) thrift

THRIFT_GO_ARGS=thrift_import="github.com/apache/thrift/lib/go/thrift"
THRIFT_PY_ARGS=tornado
THRIFT_JAVA_ARGS=private-members

THRIFT_GEN=--gen go:$(THRIFT_GO_ARGS) --gen py:$(THRIFT_PY_ARGS) --gen java:$(THRIFT_JAVA_ARGS) --gen js:node --gen cpp
THRIFT_CMD=$(THRIFT) -o /data $(THRIFT_GEN)

THRIFT_FILES=agent.thrift jaeger.thrift sampling.thrift zipkincore.thrift crossdock/tracetest.thrift \
	baggage.thrift dependency.thrift aggregation_validator.thrift

test-ci: thrift

clean:
	rm -rf gen-* || true

thrift:	thrift-image clean $(THRIFT_FILES)

$(THRIFT_FILES):
	@echo Compiling $@
	$(THRIFT_CMD) /data/thrift/$@

thrift-image:
	docker pull $(THRIFT_IMG)
	$(THRIFT) -version

.PHONY: test-ci clean thrift thrift-image $(THRIFT_FILES)
