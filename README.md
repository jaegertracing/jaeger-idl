# jaeger-idl [![Build Status][ci-img]][ci] [![Coverage Status][cov-img]][cov]

A set of shared Thrift and Protobuf data model definitions used by the Jaeger components.

As of Jan 2025 this repository also hosts Go code:
  * implementation of Jaeger-v1 domain model
    * Previous import path `"github.com/jaegertracing/jaeger/model"`
    * New import part is `"github.com/jaegertracing/jaeger-idl/model/v1"`
  * `protoc`-generated Go types for `api_v2`
    * Previous import path `"github.com/jaegertracing/jaeger/proto-gen/api_v2"`
    * New import part is `"github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"`
  * All Thrift-generated types
    * Previous import path `"github.com/jaegertracing/jaeger/thrift-gen/{agent,jaeger,sampling,zipkincore}"`
    * New import part is `"github.com/jaegertracing/jaeger-idl/thrift-gen/..."`

## Generating code

This repository only include Go code for some of the Thrift and Protobuf modules. It also runs Thrift and `protoc` generators as part of the CI to verify all IDL files. See the [Makefile](./Makefile) for example. In particular, the classes for different languages can be compiled using the `jaegertracing/protobuf` Docker image (see [README](https://github.com/jaegertracing/docker-protobuf/blob/master/README.md)).

To generate the stubs for your own purposes:
  * clone the repository
  * run `make proto-all` or `make thrift-all`
  * the stubs will be generated under `.thrift-gen-polyglot/gen-{lang}` for Thrift and `.proto-gen-polyglot/{lang}` for Protobuf


## Compatibility

The Jaeger repositories that use these IDL files usually import this repository as a Git submodule, so you can verify which revision of the IDLs is being used by looking at the submodule commit sha, e.g. in GitHub it will show like this `[->] idl @ d64c4eb`.

## Contributing

See [CONTRIBUTING](./CONTRIBUTING.md).

## License

[Apache 2.0 License](./LICENSE).


[ci-img]: https://github.com/jaegertracing/jaeger-idl/actions/workflows/ci-unit-tests.yml/badge.svg
[ci]: https://github.com/jaegertracing/jaeger-idl/actions/workflows/ci-unit-tests.yml
[cov-img]: https://codecov.io/gh/jaegertracing/jaeger-idl/branch/main/graph/badge.svg
[cov]: https://codecov.io/gh/jaegertracing/jaeger-idl/branch/main/
