# Copyright (c) 2023 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# Generate gogo, swagger, go-validators, gRPC-storage-plugin output.
#
# -I declares import folders, in order of importance. This is how proto resolves the protofile imports.
# It will check for the protofile relative to each of thesefolders and use the first one it finds.
#
# --gogo_out generates GoGo Protobuf output with gRPC plugin enabled.
# --govalidators_out generates Go validation files for our messages types, if specified.
#
# The lines starting with Mgoogle/... are proto import replacements,
# which cause the generated file to import the specified packages
# instead of the go_package's declared by the imported protof files.
#

DOCKER=docker
DOCKER_PROTOBUF_VERSION=0.5.0
DOCKER_PROTOBUF=jaegertracing/protobuf:$(DOCKER_PROTOBUF_VERSION)
PROTOC := ${DOCKER} run --rm -u ${shell id -u} -v${PWD}:${PWD} -w${PWD} ${DOCKER_PROTOBUF} --proto_path=${PWD}

PATCHED_OTEL_PROTO_DIR = proto-gen/.patched-otel-proto

PROTO_INCLUDES := \
	-Iproto/api_v2 \
	-I/usr/include/github.com/gogo/protobuf

# Remapping of std types to gogo types (must not contain spaces)
PROTO_GOGO_MAPPINGS := $(shell echo \
		Mgoogle/protobuf/descriptor.proto=github.com/gogo/protobuf/types \
		Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types \
		Mgoogle/protobuf/duration.proto=github.com/gogo/protobuf/types \
		Mgoogle/protobuf/empty.proto=github.com/gogo/protobuf/types \
		Mgoogle/api/annotations.proto=github.com/gogo/googleapis/google/api \
		Mmodel.proto=github.com/jaegertracing/jaeger/model \
	| $(SED) 's/  */,/g')

# The source directory for OTLP Protobufs from the sub-sub-module.
OTEL_PROTO_SRC_DIR=opentelemetry-proto/opentelemetry/proto

# Find all OTEL .proto files, remove leading path (only keep relevant namespace dirs).
OTEL_PROTO_FILES=$(subst $(OTEL_PROTO_SRC_DIR)/,,\
   $(shell ls $(OTEL_PROTO_SRC_DIR)/{common,resource,trace}/v1/*.proto))

# Macro to execute a command passed as argument.
# DO NOT DELETE EMPTY LINE at the end of the macro, it's required to separate commands.
define exec-command
$(1)

endef

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

.PHONY: new-proto
new-proto: new-proto-api-v2

.PHONY: new-proto-api-v2
new-proto-api-v2:
	mkdir -p proto-gen/api_v2
	$(call proto_compile, proto-gen/api_v2, proto/api_v2/query.proto)
	$(call proto_compile, proto-gen/api_v2, proto/api_v2/collector.proto)
	$(call proto_compile, proto-gen/api_v2, proto/api_v2/sampling.proto)
