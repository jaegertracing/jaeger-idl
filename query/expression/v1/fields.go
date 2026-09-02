// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package expression

import "slices"

// Built-in fields are the values a span carries directly, rather than entries in one of its
// attribute maps. A FieldRef names one by giving its level and its name.
//
// The set is defined here rather than left to each backend, because which fields a query may
// name is part of the query API: a caller writes one query against Jaeger, not a different one
// per storage backend. Which of them a given backend can actually serve is the separate
// question that SearchCapabilities answers, so a field being valid here does not promise that
// every deployment can filter on it.
//
// Most fields are a field of the corresponding OTLP message. A few are derived, computed from
// the OTLP data rather than stored in it — a span's duration from its two timestamps, an
// event's offset from its span's start — and are named because they are what people actually
// filter on.
//
// Field names are camelCase — `startTime`, `traceID`, not the proto's `start_time_unix_nano`
// and `trace_id` — because that is how proto3 JSON renders a message field, and how this API's
// own query parameters are already named, so a field reads like the rest of the JSON surface.
// The operators are snake_case (`not_in`) without contradicting that: those are values, not
// field names.
//
// A constant compared against a field is written the way that field's values are written, and
// for the two that measure time that means two different formats, neither of them a bare
// number in an assumed unit:
//
//   - A duration — `duration`, `timeSinceStart` — carries its unit, in Go duration syntax:
//     "2s", "1h30m", "50us". This is what `duration_min`/`duration_max` have always accepted,
//     and RFC 0005 §5.3 requires the unit rather than leaving nanoseconds and milliseconds to
//     be guessed at.
//   - A timestamp — `startTime`, `endTime`, `time` — is RFC 3339 with nanosecond precision:
//     "2026-08-16T18:56:20.123456789Z". That is what api_v3 already accepts for the query's
//     own time range, so a caller writes an instant one way throughout.
//
// Each level has its own vocabulary, so the constants are named for the level they belong to:
// a span's startTime and an event's time are different fields, and the three levels that each
// have a `name` have three different fields under that one name.
const (
	SpanFieldTraceID       = "traceID"
	SpanFieldSpanID        = "spanID"
	SpanFieldParentSpanID  = "parentSpanID"
	SpanFieldTraceState    = "traceState"
	SpanFieldName          = "name"
	SpanFieldKind          = "kind"
	SpanFieldStartTime     = "startTime"
	SpanFieldEndTime       = "endTime"
	SpanFieldDuration      = "duration"
	SpanFieldStatus        = "status"
	SpanFieldStatusMessage = "statusMessage"

	ResourceFieldService   = "service"
	ResourceFieldSchemaURL = "schemaURL"

	ScopeFieldName      = "name"
	ScopeFieldVersion   = "version"
	ScopeFieldSchemaURL = "schemaURL"

	EventFieldName           = "name"
	EventFieldTime           = "time"
	EventFieldTimeSinceStart = "timeSinceStart"

	LinkFieldTraceID    = "traceID"
	LinkFieldSpanID     = "spanID"
	LinkFieldTraceState = "traceState"
)

// FieldType is the type a built-in field holds, and so the type a constant compared against
// that field is read as. It is the type an unconstrained constant is resolved into, and what
// makes `span.duration > "banana"` refusable at the query boundary.
//
// It is a smaller vocabulary than it might be, because the fields below are the only ones that
// exist: IDs, a status, a span kind and a trace state are all text this API checks, and a
// distinct type only pays once something wants the parsed form (RFC 0005 §5.4). A level gains
// numeric fields the day one is defined, and the type for it is added here with the rule that
// parses it.
type FieldType string

const (
	FieldTypeString    FieldType = "string"
	FieldTypeDuration  FieldType = "duration"
	FieldTypeTimestamp FieldType = "timestamp"
	// FieldTypeSpanKind and FieldTypeSpanStatus hold one of a closed set of words, so a
	// constant compared against one is refused unless it is a member. An ID is a string
	// rather than a type of its own: a span kind outside the set can never match any span,
	// while an ID nobody recorded is indistinguishable from one the caller is looking for.
	FieldTypeSpanKind   FieldType = "spanKind"
	FieldTypeSpanStatus FieldType = "spanStatus"
)

// fieldTypes is every declared field type, walked by a test so that a field declaring a type
// this vocabulary does not list fails there rather than reaching a consumer.
var fieldTypes = []FieldType{
	FieldTypeString, FieldTypeDuration, FieldTypeTimestamp,
	FieldTypeSpanKind, FieldTypeSpanStatus,
}

// The words span.kind and span.status hold. They are lower case, like the operators and levels
// and unlike OTLP's own SPAN_KIND_SERVER, so that one vocabulary reads as one vocabulary
// (RFC 0005 §6.2). A backend maps them to whatever it stored.
var (
	spanKinds    = []string{"unspecified", "internal", "server", "client", "producer", "consumer"}
	spanStatuses = []string{"unset", "ok", "error"}
)

// SpanKinds returns the words span.kind holds, in the order OTLP declares them. It returns a copy,
// like Fields, because the validator reads the same words: a caller that could append one would
// change what every filter after it means.
func SpanKinds() []string {
	return slices.Clone(spanKinds)
}

// SpanStatuses returns the words span.status holds. It returns a copy, for the reason SpanKinds
// does.
func SpanStatuses() []string {
	return slices.Clone(spanStatuses)
}

// Field is a built-in field: its name paired with the level it belongs to, and the type it
// holds. The name and level travel together because neither identifies a field on its own —
// `name` is a field of the span, the event and the instrumentation scope alike, and `traceID`
// of the span and the link.
type Field struct {
	// A Field is built with keyed literals only, which is what the unexported member enforces:
	// a later release may declare more about a field here, and that has to stay an additive
	// change once this package is public API.
	_ struct{}

	Level Level
	Name  string
	// Type is what a constant compared against this field is read as.
	Type FieldType
	// Derived is true when the field is computed from the OTLP data rather than being a field
	// of it. A backend has to be able to compute it to serve a predicate on it, which is why
	// it is worth knowing apart.
	Derived bool
}

// fields enumerates every built-in field, level by level. A level's own attribute map is not
// listed: an attribute is named by an AttributeRef, not as a field.
var fields = []Field{
	// Span — opentelemetry.proto.trace.v1.Span. The IDs are hex, the kind and the status are
	// their OTLP names ("client", "error"), and all of them are compared as text.
	{Level: LevelSpan, Name: SpanFieldTraceID, Type: FieldTypeString},
	{Level: LevelSpan, Name: SpanFieldSpanID, Type: FieldTypeString},
	{Level: LevelSpan, Name: SpanFieldParentSpanID, Type: FieldTypeString},
	{Level: LevelSpan, Name: SpanFieldTraceState, Type: FieldTypeString},
	{Level: LevelSpan, Name: SpanFieldName, Type: FieldTypeString},
	{Level: LevelSpan, Name: SpanFieldKind, Type: FieldTypeSpanKind},
	{Level: LevelSpan, Name: SpanFieldStartTime, Type: FieldTypeTimestamp},
	{Level: LevelSpan, Name: SpanFieldEndTime, Type: FieldTypeTimestamp},
	{Level: LevelSpan, Name: SpanFieldStatus, Type: FieldTypeSpanStatus},
	{Level: LevelSpan, Name: SpanFieldStatusMessage, Type: FieldTypeString},
	// end_time_unix_nano - start_time_unix_nano, compared as a Go duration string.
	{Level: LevelSpan, Name: SpanFieldDuration, Type: FieldTypeDuration, Derived: true},

	// Resource — opentelemetry.proto.resource.v1.Resource, which carries only attributes, plus
	// the schema URL from the enclosing ResourceSpans.
	//
	// service is the service.name attribute read as a field. It is the one attribute Jaeger
	// treats as identity rather than metadata — it names every trace in the UI and keys the
	// search index of several backends — so a query says resource.service, not a tag lookup.
	{Level: LevelResource, Name: ResourceFieldService, Type: FieldTypeString, Derived: true},
	{Level: LevelResource, Name: ResourceFieldSchemaURL, Type: FieldTypeString},

	// Scope — opentelemetry.proto.common.v1.InstrumentationScope, plus the
	// schema URL from the enclosing ScopeSpans.
	{Level: LevelScope, Name: ScopeFieldName, Type: FieldTypeString},
	{Level: LevelScope, Name: ScopeFieldVersion, Type: FieldTypeString},
	{Level: LevelScope, Name: ScopeFieldSchemaURL, Type: FieldTypeString},

	// Event — Span.Event.
	{Level: LevelEvent, Name: EventFieldName, Type: FieldTypeString},
	{Level: LevelEvent, Name: EventFieldTime, Type: FieldTypeTimestamp},
	// Event.time_unix_nano - Span.start_time_unix_nano, compared as a Go duration string.
	{Level: LevelEvent, Name: EventFieldTimeSinceStart, Type: FieldTypeDuration, Derived: true},

	// Link — Span.Link. The IDs are the linked span's, not the linking one's.
	{Level: LevelLink, Name: LinkFieldTraceID, Type: FieldTypeString},
	{Level: LevelLink, Name: LinkFieldSpanID, Type: FieldTypeString},
	{Level: LevelLink, Name: LinkFieldTraceState, Type: FieldTypeString},
}

// Fields returns every built-in field a query may name. A caller that offers fields to choose
// from — a query builder in a UI — enumerates them here rather than hard-coding a list that
// drifts.
func Fields() []Field {
	return append([]Field(nil), fields...)
}

// LookupField returns the built-in field of that level and name. Its second result is false
// when no such field is defined, which is what the query boundary refuses a filter for.
func LookupField(level Level, name string) (Field, bool) {
	for _, f := range fields {
		if f.Level == level && f.Name == name {
			return f, true
		}
	}
	return Field{}, false
}
