# How to Contribute to Jaeger

We'd love your help!

Please see the [contributing guidelines](https://github.com/jaegertracing/jaeger/blob/main/CONTRIBUTING_GUIDELINES.md) for general policies of the project.

## Prerequisites

This repository uses Proto and Thrift compilers that are already packaged as container images, so you need to have Docker or similar solution installed.

## Making changes to the .proto files

After making any changes to .proto files make sure all generated files are up to date by running:
```
make init-submodule
make proto
make proto-all
make test-code-gen
make test-ci
make lint
```

## Making changes to the .thrift files

After making any changes to .thrift files make sure all generated files are up to date by running:
```
make init-submodule
make thrift
make thrift-all
make test-code-gen
make test-ci
make lint
```
