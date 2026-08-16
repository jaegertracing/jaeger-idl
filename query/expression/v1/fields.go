// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package expression

// Built-in fields are the values a span carries directly, rather than entries in one of its
// attribute maps. A Reference names one by giving its level and leaving Attr unset.
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
// filter on. The names follow TraceQL's camelCase intrinsics so that they read familiarly to
// its users, which is why they are `startTime` and `traceID` rather than the proto's
// `start_time_unix_nano` and `trace_id`.
const (
	// Shared by more than one level, which is why the level is always given separately: a span
	// name and an event name are different fields that spell their name the same way.
	FieldName       = "name"
	FieldTraceID    = "traceID"
	FieldSpanID     = "spanID"
	FieldTraceState = "traceState"
	FieldSchemaURL  = "schemaURL"

	// Span.
	FieldParentSpanID  = "parentSpanID"
	FieldKind          = "kind"
	FieldStartTime     = "startTime"
	FieldEndTime       = "endTime"
	FieldDuration      = "duration"
	FieldStatus        = "status"
	FieldStatusMessage = "statusMessage"

	// Resource.
	FieldService = "service"

	// Instrumentation scope.
	FieldVersion = "version"

	// Event.
	FieldTime           = "time"
	FieldTimeSinceStart = "timeSinceStart"
)

// Field is a built-in field: its name paired with the level it belongs to. The two travel
// together because neither identifies a field on its own — `name` is a field of the span, the
// event and the instrumentation scope alike, and `traceID` of the span and the link.
type Field struct {
	Level Level
	Name  string
	// Derived is true when the field is computed from the OTLP data rather than being a field
	// of it. A backend has to be able to compute it to serve a predicate on it, which is why
	// it is worth knowing apart.
	Derived bool
}

// fields enumerates every built-in field, level by level. A level's own attribute map is not
// listed: an attribute is reached with Attr set, not as a field.
var fields = []Field{
	// Span — opentelemetry.proto.trace.v1.Span.
	{Level: LevelSpan, Name: FieldTraceID},
	{Level: LevelSpan, Name: FieldSpanID},
	{Level: LevelSpan, Name: FieldParentSpanID},
	{Level: LevelSpan, Name: FieldTraceState},
	{Level: LevelSpan, Name: FieldName},
	{Level: LevelSpan, Name: FieldKind},
	{Level: LevelSpan, Name: FieldStartTime},
	{Level: LevelSpan, Name: FieldEndTime},
	{Level: LevelSpan, Name: FieldStatus},
	{Level: LevelSpan, Name: FieldStatusMessage},
	// end_time_unix_nano - start_time_unix_nano.
	{Level: LevelSpan, Name: FieldDuration, Derived: true},

	// Resource — opentelemetry.proto.resource.v1.Resource, which carries only attributes, plus
	// the schema URL from the enclosing ResourceSpans.
	//
	// service is the service.name attribute read as a field. It is the one attribute Jaeger
	// treats as identity rather than metadata — it names every trace in the UI and keys the
	// search index of several backends — so a query says resource.service, not a tag lookup.
	{Level: LevelResource, Name: FieldService, Derived: true},
	{Level: LevelResource, Name: FieldSchemaURL},

	// Instrumentation scope — opentelemetry.proto.common.v1.InstrumentationScope, plus the
	// schema URL from the enclosing ScopeSpans.
	{Level: LevelInstrumentation, Name: FieldName},
	{Level: LevelInstrumentation, Name: FieldVersion},
	{Level: LevelInstrumentation, Name: FieldSchemaURL},

	// Event — Span.Event.
	{Level: LevelEvent, Name: FieldName},
	{Level: LevelEvent, Name: FieldTime},
	// Event.time_unix_nano - Span.start_time_unix_nano.
	{Level: LevelEvent, Name: FieldTimeSinceStart, Derived: true},

	// Link — Span.Link. The IDs are the linked span's, not the linking one's.
	{Level: LevelLink, Name: FieldTraceID},
	{Level: LevelLink, Name: FieldSpanID},
	{Level: LevelLink, Name: FieldTraceState},
}

// Fields returns every built-in field a query may name. A caller that offers fields to choose
// from — a query builder in a UI — enumerates them here rather than hard-coding a list that
// drifts.
func Fields() []Field {
	return append([]Field(nil), fields...)
}

// LookupField returns the built-in field of that level and name. Its second result is false
// when no such field is defined, which is what ValidateFilter refuses.
func LookupField(level Level, name string) (Field, bool) {
	for _, f := range fields {
		if f.Level == level && f.Name == name {
			return f, true
		}
	}
	return Field{}, false
}
