FROM golang:1.13.2

ENV PROTOBUF_VERSION="3.10.0"
ENV PATH="/opt/protoc/bin:${PATH}"

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
  go get -u github.com/gogo/protobuf/protoc-gen-gogo; \
  go get -u github.com/golang/protobuf/protoc-gen-go; \
  mkdir -p "${GOPATH}/src/github.com/googleapis"; \
  git clone https://github.com/googleapis/googleapis.git "${GOPATH}/src/github.com/googleapis/googleapis";

# Do Properly
WORKDIR /usr/src/app
CMD protoc -I=. -I=$GOPATH/src -I=$GOPATH/src/github.com/gogo/protobuf -I=$GOPATH/src/github.com/googleapis/googleapis --gofast_out=go/ --java_out=java/ --python_out=python/ proto/model.proto
