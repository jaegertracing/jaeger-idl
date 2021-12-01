# jaeger-idl [![Build Status][ci-img]][ci]

A set of shared Thrift and Protobuf data model definitions used by the Jaeger components.

## Generating code

This repository does not publish the generated code, but it does run Thrift and `protoc` generators as part of the CI to verify all IDL files. See the [Makefile](./Makefile) for example. In particular, the classes for different languages can be compiled using the `jaegertracing/protobuf` Docker image (see [README](https://github.com/jaegertracing/docker-protobuf/blob/master/README.md)).

To generate the stubs for your own purposes:
  * clone the repository
  * run `make proto` or `make thrift`
  * the stubs will be generated under `gen-{lang}` for Thrift and `proto-gen-{lang}` for Protobuf


## Compatibility

The Jaeger repositories that use these IDL files usually import this repository as a Git submodule, so you can verify which revision of the IDLs is being used by looking at the submodule commit sha, e.g. in GitHub it will show like this `[->] idl @ d64c4eb`.

## Contributing

See [CONTRIBUTING](./CONTRIBUTING.md).

## License
  
[Apache 2.0 License](./LICENSE).


[ci-img]: https://github.com/jaegertracing/jaeger-idl/workflows/Tests/badge.svg?branch=main
[ci]: https://github.com/jaegertracing/jaeger-idl/actions?query=branch%3Amain
