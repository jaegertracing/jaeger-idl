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
// filter on.
//
// Field names are camelCase — `startTime`, `traceID`, not the proto's `start_time_unix_nano`
// and `trace_id` — because that is how proto3 JSON renders a message field, and how this API's
// own query parameters are already spelled, so a field reads like the rest of the JSON surface.
// The operators are snake_case (`not_in`) without contradicting that: those are values, not
// field names.
//
// A constant compared against a field is written the way that field's values are written, and
// for the two that measure time that means two different spellings, neither of them a bare
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
// have a `name` have three different fields that spell it the same way.
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
	{Level: LevelSpan, Name: SpanFieldTraceID},
	{Level: LevelSpan, Name: SpanFieldSpanID},
	{Level: LevelSpan, Name: SpanFieldParentSpanID},
	{Level: LevelSpan, Name: SpanFieldTraceState},
	{Level: LevelSpan, Name: SpanFieldName},
	{Level: LevelSpan, Name: SpanFieldKind},
	{Level: LevelSpan, Name: SpanFieldStartTime},
	{Level: LevelSpan, Name: SpanFieldEndTime},
	{Level: LevelSpan, Name: SpanFieldStatus},
	{Level: LevelSpan, Name: SpanFieldStatusMessage},
	// end_time_unix_nano - start_time_unix_nano, compared as a Go duration string.
	{Level: LevelSpan, Name: SpanFieldDuration, Derived: true},

	// Resource — opentelemetry.proto.resource.v1.Resource, which carries only attributes, plus
	// the schema URL from the enclosing ResourceSpans.
	//
	// service is the service.name attribute read as a field. It is the one attribute Jaeger
	// treats as identity rather than metadata — it names every trace in the UI and keys the
	// search index of several backends — so a query says resource.service, not a tag lookup.
	{Level: LevelResource, Name: ResourceFieldService, Derived: true},
	{Level: LevelResource, Name: ResourceFieldSchemaURL},

	// Scope — opentelemetry.proto.common.v1.InstrumentationScope, plus the
	// schema URL from the enclosing ScopeSpans.
	{Level: LevelScope, Name: ScopeFieldName},
	{Level: LevelScope, Name: ScopeFieldVersion},
	{Level: LevelScope, Name: ScopeFieldSchemaURL},

	// Event — Span.Event.
	{Level: LevelEvent, Name: EventFieldName},
	{Level: LevelEvent, Name: EventFieldTime},
	// Event.time_unix_nano - Span.start_time_unix_nano, compared as a Go duration string.
	{Level: LevelEvent, Name: EventFieldTimeSinceStart, Derived: true},

	// Link — Span.Link. The IDs are the linked span's, not the linking one's.
	{Level: LevelLink, Name: LinkFieldTraceID},
	{Level: LevelLink, Name: LinkFieldSpanID},
	{Level: LevelLink, Name: LinkFieldTraceState},
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
