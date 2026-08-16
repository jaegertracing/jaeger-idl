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
// spellings of the same tree. Converting to and from the wire is the business of whoever
// owns a wire.
package expression

// Level is the scope a referenced value lives in. The five explicit levels are the
// OTLP attribute maps; an empty Level means an unqualified attribute, searched at the
// span and resource levels. See RFC 0005 §5.1.
type Level string

const (
	LevelSpan            Level = "span"
	LevelResource        Level = "resource"
	LevelInstrumentation Level = "instrumentation"
	LevelEvent           Level = "event"
	LevelLink            Level = "link"
)

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

// ValueType is the declared type of a constant. It is optional: empty means the
// backend matches the value at whatever type it was stored, and a type that is set is
// authoritative, so the backend matches only values of that type. See RFC 0005 §5.4.
type ValueType string

const (
	ValueTypeString ValueType = "string"
	ValueTypeInt    ValueType = "int"
	ValueTypeDouble ValueType = "double"
	ValueTypeBool   ValueType = "bool"
)

// Expression is a node in a structured filter: an atom — a Reference to a value on
// the span, or a Scalar or List constant — or a Call applying an operator to argument
// expressions. Only the four types in this package implement it, so a backend can
// switch on the concrete type and cover every case. See RFC 0005 §6.
type Expression interface {
	isExpression()
}

// Reference names a value on the span. At an explicit Level, Attr chooses between the
// built-in field called Name and the entry keyed by Name in that level's attribute
// map. An empty Level is always an attribute, whatever Attr says.
type Reference struct {
	// Name is empty only for the collection itself — an event- or link-level Reference
	// standing for every event or link of the span, which is what OpSome quantifies over.
	Name string
	// Level is empty (unqualified) or one of the five explicit levels.
	Level Level
	// Attr is true for an attribute of Level, false for its built-in field.
	Attr bool
}

// IsAttribute reports whether r names an entry in an attribute map rather than a built-in
// field. It is not the Attr bit on its own: an unqualified reference is an attribute of the
// span or resource however Attr is set, because no built-in field has an unqualified form.
func (r *Reference) IsAttribute() bool {
	return r.Level == "" || r.Attr
}

// IsField reports whether r references the built-in field of that level and name. Both are
// given because neither identifies a field alone (see Field), and an attribute never matches
// however it is spelled: level and name are not enough to tell a field from a tag that borrows
// its spelling.
func (r *Reference) IsField(level Level, name string) bool {
	return !r.IsAttribute() && r.Level == level && r.Name == name
}

// Scalar is a single constant value. The value is carried as a string whatever its
// Type, because a value with a unit — a duration such as "2s" — has no native scalar
// form.
type Scalar struct {
	Value string
	Type  ValueType
}

// List is a homogeneous list constant, the right-hand argument of OpIn and OpNotIn.
// Type applies to every element.
type List struct {
	Values []string
	Type   ValueType
}

// Call applies Op to Args. The arity follows the operator: OpNot and OpExists are
// unary, the comparisons and OpIn/OpNotIn are binary, and OpAnd/OpOr take two or
// more. Because an argument is itself an Expression, a predicate can compare two
// references as readily as a reference against a constant.
type Call struct {
	Op   Operator
	Args []Expression
}

func (*Reference) isExpression() {}
func (*Scalar) isExpression()    {}
func (*List) isExpression()      {}
func (*Call) isExpression()      {}
