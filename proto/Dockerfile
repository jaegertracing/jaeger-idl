FROM golang:1.13.2

ENV PROTOBUF_VERSION="3.10.0"
ENV PATH="/opt/protoc/bin:${PATH}"

# We need the binaries from protoc-gen-gogo, so we install it, but just download
# the other repos.
# googleapis.git has to be downloaded with git, because it has no go files.
RUN set -ex; \
  apt-get update; \
  apt-get install -y \
    -o APT::Install-Recommends=false \
    -o APT::Install-Suggests=false \
    unzip; \
  rm -rf /var/lib/apt/lists/*; \
  wget https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOBUF_VERSION}/protoc-${PROTOBUF_VERSION}-linux-x86_64.zip; \
  unzip -d /opt/protoc protoc-${PROTOBUF_VERSION}-linux-x86_64.zip; \
  apt-get purge -y --auto-remove unzip; \
  GO111MODULE=on go get -u \
    github.com/gogo/protobuf/protoc-gen-gogo; \
  GO111MODULE=on go get -d -u \
    github.com/grpc-ecosystem/grpc-gateway; \
  mkdir -p "${GOPATH}/src/github.com/googleapis"; \
  git clone --depth 1 https://github.com/googleapis/googleapis.git "${GOPATH}/src/github.com/googleapis/googleapis";

# Do Properly
WORKDIR /work
ENTRYPOINT [ \
  "protoc", \
  "-I=/go/pkg/mod/github.com/gogo/protobuf@v1.3.1/", \
  "-I=/go/src/github.com/googleapis/googleapis", \
  "-I=/go/pkg/mod/github.com/grpc-ecosystem/grpc-gateway@v1.11.3/" \
]
CMD [ \
  "-I=./proto", \
  "--gogo_out=plugins=grpc,Mgoogle/protobuf/descriptor.proto=github.com/gogo/protobuf/types,Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types,Mgoogle/protobuf/duration.proto=github.com/gogo/protobuf/types,Mgoogle/protobuf/empty.proto=github.com/gogo/protobuf/types,Mgoogle/api/annotations.proto=github.com/gogo/googleapis/google/api,Mmodel.proto=github.com/jaegertracing/jaeger/model:go/", \
  "proto/api_v2/sampling.proto", \
  "proto/api_v2/query.proto", \
  "proto/api_v2/collector.proto" \
]
