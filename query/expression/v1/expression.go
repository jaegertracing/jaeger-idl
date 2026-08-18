// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package expression defines the structured trace-query filter of Jaeger RFC 0005,
// "Structured Query Filters for Trace Search":
// https://github.com/jaegertracing/jaeger/blob/main/docs/rfc/0005-structured-query-filters.md
// Section references (§N) below point into that document.
//
// These are plain Go types, deliberately independent of the proto definitions the filter
// travels over the wire in. The AST is the contract, and it is one contract: the same types
// describe a filter arriving on the public query API, reaching a storage backend, and
// being gated by a query interceptor, so nothing in that path has to translate between two
// representations of the same tree. Converting to and from the wire is the business of whoever
// owns a wire.
package expression

import "time"

// Level is the scope a referenced value lives in. The five levels are the OTLP attribute
// maps; an attribute reference may also leave it empty, which searches the span and
// resource levels. See RFC 0005 §5.1.
type Level string

const (
	LevelSpan     Level = "span"
	LevelResource Level = "resource"
	LevelScope    Level = "scope"
	LevelEvent    Level = "event"
	LevelLink     Level = "link"
)

// levels is every explicit level. Validation walks it rather than repeating the constants in a
// switch, so the vocabulary has one definition and a test can enumerate what is accepted.
var levels = []Level{LevelSpan, LevelResource, LevelScope, LevelEvent, LevelLink}

// Operator is what a Call applies to its arguments: a boolean combinator, a
// comparison, a set-membership test, or the existential quantifier over a span's
// events or links. See RFC 0005 §5.3 and §5.5.
type Operator string

const (
	OpAnd    Operator = "and"
	OpOr     Operator = "or"
	OpNot    Operator = "not"
	OpEq     Operator = "eq"
	OpNe     Operator = "ne"
	OpGt     Operator = "gt"
	OpLt     Operator = "lt"
	OpGte    Operator = "gte"
	OpLte    Operator = "lte"
	OpRegex  Operator = "regex"
	OpExists Operator = "exists"
	OpIn     Operator = "in"
	OpNotIn  Operator = "not_in"
	OpSome   Operator = "some"
)

// operators is every operator. Nothing dispatches on it — validateCall has a case per operator
// — but a test walks it to catch an operator added without a case, which would otherwise be
// reported as unknown.
var operators = []Operator{
	OpAnd, OpOr, OpNot,
	OpEq, OpNe, OpGt, OpLt, OpGte, OpLte, OpRegex, OpExists, OpIn, OpNotIn,
	OpSome,
}

// ValueType is the type a constant declares on the wire. It is optional there: empty means
// the backend matches the value at whatever type it was stored, and a type that is set is
// authoritative, so the backend matches only values of that type. In this AST a constant is a
// typed node instead (see AnyValue and the values beside it), so the vocabulary is left for
// List, whose elements are strings, and for whoever converts a wire message into a node.
// See RFC 0005 §5.4.
type ValueType string

const (
	ValueTypeString ValueType = "string"
	ValueTypeInt    ValueType = "int"
	ValueTypeDouble ValueType = "double"
	ValueTypeBool   ValueType = "bool"
)

// valueTypes is every type a constant may declare. An empty type is always allowed and is not
// listed, because it means "any type" rather than a type.
var valueTypes = []ValueType{ValueTypeString, ValueTypeInt, ValueTypeDouble, ValueTypeBool}

// Expression is a node in a structured filter: an atom — a reference to a value on the
// span, or a constant — or a Call applying an operator to argument expressions. Only the
// types in this package implement it, so a backend can switch on the concrete type and
// cover every case. See RFC 0005 §6.
type Expression interface {
	isExpression()
}

// expressionTerm is embedded by each term type, which is how a type says it is one: it shows
// in the declaration rather than in a marker method further down the file. Being unexported is
// what closes the interface, since no other package can embed it. It also means a term is
// built with keyed literals only, so a later release can add a field to one.
//
// The receiver is a pointer so that only the pointer types satisfy Expression, not the values.
// A tree is built from pointers throughout, and a value that also satisfied the interface would
// slip past every type switch written for the pointer.
type expressionTerm struct{}

func (*expressionTerm) isExpression() {}

// AttributeRef names an entry in one of the span's attribute maps. See RFC 0005 §5.1.
type AttributeRef struct {
	expressionTerm

	Key string
	// Level is empty for the unqualified span-or-resource search, or one of the five levels.
	Level Level
}

// FieldRef names a built-in field — a value the data model defines directly rather than an
// attribute-map entry, such as a span's duration. Level is never empty. See RFC 0005 §5.2
// and Field.
type FieldRef struct {
	expressionTerm

	Name  string
	Level Level
}

// NestedRef names a span's events or links collection, which is what OpSome quantifies over and
// the only place it may appear. See RFC 0005 §5.5.
type NestedRef struct {
	expressionTerm

	Level Level
}

// AnyValue is a constant under no type constraint: the caller wrote a value and said nothing
// about how to read it, so a backend matches it at whatever type the value was stored. It is
// also what an unhinted duration or timestamp arrives as, until it is resolved against the
// field it is compared with (see ResolveConstants).
type AnyValue struct {
	expressionTerm

	Value string
}

// StringValue is a constant to be matched as text.
type StringValue struct {
	expressionTerm

	Value string
}

// IntValue is a constant to be matched as an integer.
type IntValue struct {
	expressionTerm

	Value int64
}

// DoubleValue is a constant to be matched as a floating-point number.
type DoubleValue struct {
	expressionTerm

	Value float64
}

// BoolValue is a constant to be matched as a boolean.
type BoolValue struct {
	expressionTerm

	Value bool
}

// DurationValue is a length of time, which is what a duration field is compared against.
//
// The wire has no duration type. One travels as an unhinted constant written in Go duration
// syntax, "2s" or "50us", so this node is reached by resolving that constant against the field
// it is compared to (see ResolveConstants).
//
// Beside an attribute reference there is no field to resolve against, so a round trip through
// the wire hands the receiver an AnyValue holding the same text. Nothing is lost that an
// attribute had to begin with: only storage knows what type it was written as.
type DurationValue struct {
	expressionTerm

	Value time.Duration
}

// TimestampValue is an instant, which is what a timestamp field is compared against.
//
// Like DurationValue it has no wire type of its own, arrives as an unhinted RFC 3339 constant,
// and comes back as an AnyValue when it travels beside an attribute.
type TimestampValue struct {
	expressionTerm

	Value time.Time
}

// List is a homogeneous list constant, the right-hand argument of OpIn and OpNotIn. Its elements
// stay as the caller wrote them, and every one of them is read as a single type.
//
// That type is always known: Type declares it, or the built-in field the list is compared against
// supplies it. Compared against an attribute, which declares nothing itself, the list has to
// declare it — and it is worth declaring anyway, because a list matches only values of the type it
// names.
//
// Unlike a constant, a list is not rewritten into typed elements when a filter is finalized. Two
// reasons. A backend that indexes a value as text matches the text a caller wrote, and re-writing
// "1.50" as a number and back would hand it "1.5" instead. And an element that cannot be read as the
// list's type is refused while finalizing, so what a consumer holds is text already known to be
// readable: ReadElement turns one into the typed node, and nothing has to parse it defensively.
type List struct {
	expressionTerm

	Values []string
	Type   ValueType
}

// Call applies Op to Args. The arity follows the operator: OpNot and OpExists are
// unary, the comparisons and OpIn/OpNotIn are binary, and OpAnd/OpOr take two or
// more. Because an argument is itself an Expression, a comparison reads two
// references as readily as a reference and a constant — what it requires is that
// both operands hold the same kind of value (see ValidateFilter).
type Call struct {
	expressionTerm

	Op   Operator
	Args []Expression
}
