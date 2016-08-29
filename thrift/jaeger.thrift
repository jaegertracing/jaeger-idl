# Copyright (c) 2016 Uber Technologies, Inc.
#
# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included in
# all copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
# THE SOFTWARE.

namespace java com.uber.jaeger.thriftjava

# TagType denotes the type of a Tag's value.
enum TagType { STRING, DOUBLE, BOOL, I16, I32, I64, BINARY }

# Tag is a basic strongly typed key/value pair. It has been flattened to reduce the use of pointers in golang
struct Tag {
  1: required string   key
  2: required TagType tagType
  3: required string  vStr
  4: required double  vDouble
  5: required bool    vBool
  6: required i16     vInt16
  7: required i32     vInt32
  8: required i64     vInt64
  9: required binary  vBinary
}

# Log is a timed even with an arbitrary set of tags.
struct Log {
  1: required i64       timestamp
  2: required list<Tag> tags
}

enum SpanRefType { CHILD_OF, FOLLOWS_FROM }

# SpanRef describes causal relationship of the current span to another span (e.g. 'child-of')
struct SpanRef {
  1: required SpanRefType refType
  2: required i64         traceId
  3: required i64         spanId
}

# Span represents a named unit of work performed by a service.
struct Span {
  1: required i64           traceId      # unique trace id, the same for all spans in the trace
  2: required i64           spanId       # unique span id (only unique within a given trace)
  3: required string        operationName
  4: optional list<SpanRef> references    # causal references to other spans
  5: required i32           flags         # tbd
  6: required i64           startTime
  7: required i64           duration
  8: optional list<Tag>     tags
  9: optional list<Log>     logs
}

# Process describes the traced process/service that emits spans.
struct Process {
  1: required string    serviceName
  2: optional list<Tag> tags
}

# Batch is a collection of spans reported out of process.
struct Batch {
  1: required Process    process
  2: required list<Span> spans
}

service Agent {
    oneway void emitJaegerBatch(1: list<Span> spans)
}
