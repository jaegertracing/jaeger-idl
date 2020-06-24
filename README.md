# jaeger-idl [![Build Status][ci-img]][ci]

A set of shared data model definitions used by Jaeger components.

## Generating code

This repository does not publish the generated code, but it does run Thrift and `protoc` generators as part of the CI to verify all IDL files. See the [Makefile](./Makefile) for example. In particular, the classes for different languages can be compiled using the `jaegertracing/protobuf` Docker image (see [README](https://github.com/jaegertracing/docker-protobuf/blob/master/README.md)).

## Compatibility

The Jaeger repositories that use these IDL files usually import this repository as a Git submodule, so you can verify which revision of the IDLs is being used by looking at the submodule commit sha, e.g. in GitHub it will show like this `[->] idl @ d64c4eb`.

## Contributing

See [CONTRIBUTING](./CONTRIBUTING.md).

## License
  
[Apache 2.0 License](./LICENSE).


[ci-img]: https://travis-ci.org/jaegertracing/jaeger-idl.svg?branch=master
[ci]: https://travis-ci.org/jaegertracing/jaeger-idl
