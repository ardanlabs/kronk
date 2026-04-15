// Package jinja implements a Jinja template engine purpose-built for LLM
// chat templates.
package jinja

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// =============================================================================

// Kind represents the type of a template value.
type Kind int

const (
	KindUndefined Kind = iota
	KindNone
	KindBool
	KindInt
	KindFloat
	KindString
	KindList
	KindDict
	KindCallable
)

// =============================================================================

// Value is a tagged union over JSON-like types used throughout the template
// engine. The v field holds one of: bool, int64, float64, string, *List,
// *Dict, or Callable.
type Value struct {
	kind Kind
	v    any
}

// Undefined returns an undefined value.
func Undefined() Value {
	return Value{kind: KindUndefined}
}

// None returns a none value.
func None() Value {
	return Value{kind: KindNone}
}

// NewBool returns a boolean value.
func NewBool(b bool) Value {
	return Value{kind: KindBool, v: b}
}

// NewInt returns an integer value.
func NewInt(n int64) Value {
	return Value{kind: KindInt, v: n}
}

// NewFloat returns a floating-point value.
func NewFloat(f float64) Value {
	return Value{kind: KindFloat, v: f}
}

// NewString returns a string value.
func NewString(s string) Value {
	return Value{kind: KindString, v: s}
}

// NewList returns a list value containing the provided items.
func NewList(items []Value) Value {
	return Value{kind: KindList, v: &List{Items: items}}
}

// NewDict returns an empty dict value.
func NewDict() Value {
	return Value{kind: KindDict, v: &Dict{Data: make(map[string]Value)}}
}

// NewCallable returns a callable value.
func NewCallable(name string, fn func(args []Value, kwargs map[string]Value) (Value, error)) Value {
	return Value{kind: KindCallable, v: Callable{Name: name, Fn: fn}}
}

// =============================================================================

// IsUndefined reports whether v is undefined.
func (v Value) IsUndefined() bool { return v.kind == KindUndefined }

// IsNone reports whether v is none.
func (v Value) IsNone() bool { return v.kind == KindNone }

// IsBool reports whether v is a boolean.
func (v Value) IsBool() bool { return v.kind == KindBool }

// IsInt reports whether v is an integer.
func (v Value) IsInt() bool { return v.kind == KindInt }

// IsFloat reports whether v is a float.
func (v Value) IsFloat() bool { return v.kind == KindFloat }

// IsNumber reports whether v is an integer or float.
func (v Value) IsNumber() bool { return v.kind == KindInt || v.kind == KindFloat }

// IsString reports whether v is a string.
func (v Value) IsString() bool { return v.kind == KindString }

// IsList reports whether v is a list.
func (v Value) IsList() bool { return v.kind == KindList }

// IsDict reports whether v is a dict.
func (v Value) IsDict() bool { return v.kind == KindDict }

// IsCallable reports whether v is callable.
func (v Value) IsCallable() bool { return v.kind == KindCallable }

// IsTruthy returns the Python-style truthiness of the value.
func (v Value) IsTruthy() bool {
	switch v.kind {
	case KindUndefined:
		return false
	case KindNone:
		return false
	case KindBool:
		return v.v.(bool)
	case KindInt:
		return v.v.(int64) != 0
	case KindFloat:
		return v.v.(float64) != 0.0
	case KindString:
		return len(v.v.(string)) > 0
	case KindList:
		return v.v.(*List).Len() > 0
	case KindDict:
		return v.v.(*Dict).Len() > 0
	case KindCallable:
		return true
	}

	return false
}

// =============================================================================

// AsBool returns the bool held by v. It panics if v is not a bool.
func (v Value) AsBool() bool { return v.v.(bool) }

// AsInt returns the int64 held by v. It panics if v is not an int.
func (v Value) AsInt() int64 { return v.v.(int64) }

// AsFloat returns the float64 held by v. It panics if v is not a float.
func (v Value) AsFloat() float64 { return v.v.(float64) }

// AsString returns the string held by v. It panics if v is not a string.
func (v Value) AsString() string { return v.v.(string) }

// AsList returns the *List held by v. It panics if v is not a list.
func (v Value) AsList() *List { return v.v.(*List) }

// AsDict returns the *Dict held by v. It panics if v is not a dict.
func (v Value) AsDict() *Dict { return v.v.(*Dict) }

// AsCallable returns the *Callable held by v. It panics if v is not callable.
func (v Value) AsCallable() *Callable {
	c := v.v.(Callable)
	return &c
}

// =============================================================================

// String returns a Python-style string representation of the value.
func (v Value) String() string {
	switch v.kind {
	case KindUndefined:
		return "Undefined"

	case KindNone:
		return "None"

	case KindBool:
		if v.v.(bool) {
			return "True"
		}
		return "False"

	case KindInt:
		return fmt.Sprintf("%d", v.v.(int64))

	case KindFloat:
		return fmt.Sprintf("%g", v.v.(float64))

	case KindString:
		return fmt.Sprintf("'%s'", v.v.(string))

	case KindList:
		list := v.v.(*List)
		var b strings.Builder
		b.WriteByte('[')
		for i, item := range list.Items {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(item.String())
		}
		b.WriteByte(']')
		return b.String()

	case KindDict:
		d := v.v.(*Dict)
		var b strings.Builder
		b.WriteByte('{')
		for i, key := range d.Keys {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "'%s': %s", key, d.Data[key].String())
		}
		b.WriteByte('}')
		return b.String()

	case KindCallable:
		c := v.v.(Callable)
		return fmt.Sprintf("<callable %s>", c.Name)
	}

	return "Undefined"
}

// Equals reports whether v and other hold the same value.
func (v Value) Equals(other Value) bool {
	if v.kind != other.kind {
		return false
	}

	switch v.kind {
	case KindUndefined, KindNone:
		return true

	case KindBool:
		return v.v.(bool) == other.v.(bool)

	case KindInt:
		return v.v.(int64) == other.v.(int64)

	case KindFloat:
		return v.v.(float64) == other.v.(float64)

	case KindString:
		return v.v.(string) == other.v.(string)

	case KindList:
		a := v.v.(*List)
		b := other.v.(*List)
		if a.Len() != b.Len() {
			return false
		}
		for i := range a.Items {
			if !a.Items[i].Equals(b.Items[i]) {
				return false
			}
		}
		return true

	case KindDict:
		a := v.v.(*Dict)
		b := other.v.(*Dict)
		if a.Len() != b.Len() {
			return false
		}
		for _, key := range a.Keys {
			bv, ok := b.Get(key)
			if !ok {
				return false
			}
			if !a.Data[key].Equals(bv) {
				return false
			}
		}
		return true

	case KindCallable:
		return false
	}

	return false
}

// =============================================================================

// List holds an ordered sequence of values.
type List struct {
	Items []Value
}

// Len returns the number of items in the list.
func (l *List) Len() int { return len(l.Items) }

// Get returns the item at position i. It panics if i is out of range.
func (l *List) Get(i int) Value { return l.Items[i] }

// Append adds a value to the end of the list.
func (l *List) Append(v Value) { l.Items = append(l.Items, v) }

// =============================================================================

// Dict is an ordered map of string keys to values. Insertion order is
// preserved via the Keys slice.
type Dict struct {
	Keys []string
	Data map[string]Value
}

// Get returns the value for key and reports whether it was found.
func (d *Dict) Get(key string) (Value, bool) {
	v, ok := d.Data[key]
	return v, ok
}

// Set inserts or updates a key-value pair. New keys are appended to the
// insertion-order list.
func (d *Dict) Set(key string, val Value) {
	if _, ok := d.Data[key]; !ok {
		d.Keys = append(d.Keys, key)
	}
	d.Data[key] = val
}

// Has reports whether key exists in the dict.
func (d *Dict) Has(key string) bool {
	_, ok := d.Data[key]
	return ok
}

// Len returns the number of entries in the dict.
func (d *Dict) Len() int { return len(d.Keys) }

// OrderedKeys returns the keys in insertion order.
func (d *Dict) OrderedKeys() []string {
	out := make([]string, len(d.Keys))
	copy(out, d.Keys)
	return out
}

// =============================================================================

// Callable represents a named callable function within the template engine.
type Callable struct {
	Name string
	Fn   func(args []Value, kwargs map[string]Value) (Value, error)
}

// =============================================================================

// FromGoValue converts a plain Go value to a Value. Supported source types
// are nil, bool, int, int64, float64, string, []any, map[string]any, and
// []Value. For map[string]any the keys are sorted for deterministic order.
// Unrecognized types produce Undefined.
func FromGoValue(v any) Value {
	switch val := v.(type) {
	case nil:
		return None()

	case bool:
		return NewBool(val)

	case int:
		return NewInt(int64(val))

	case int64:
		return NewInt(val)

	case float64:
		return NewFloat(val)

	case string:
		return NewString(val)

	case []Value:
		items := make([]Value, len(val))
		copy(items, val)
		return NewList(items)

	case []any:
		items := make([]Value, len(val))
		for i, item := range val {
			items[i] = FromGoValue(item)
		}
		return NewList(items)

	case map[string]any:
		d := NewDict()
		dict := d.AsDict()

		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			dict.Set(k, FromGoValue(val[k]))
		}
		return d
	}

	// Reflection fallback for named types (e.g., []model.D where D is
	// map[string]any). Go's type switch won't match named types against
	// their underlying type, so we use reflect to handle slices and maps
	// whose elements are convertible to any or map[string]any.
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice:
		items := make([]Value, rv.Len())
		for i := range rv.Len() {
			items[i] = FromGoValue(rv.Index(i).Interface())
		}
		return NewList(items)

	case reflect.Map:
		if rv.Type().Key().Kind() == reflect.String {
			d := NewDict()
			dict := d.AsDict()
			iter := rv.MapRange()
			keys := make([]string, 0, rv.Len())
			vals := make(map[string]any, rv.Len())
			for iter.Next() {
				k := iter.Key().String()
				keys = append(keys, k)
				vals[k] = iter.Value().Interface()
			}
			sort.Strings(keys)
			for _, k := range keys {
				dict.Set(k, FromGoValue(vals[k]))
			}
			return d
		}
	}

	return Undefined()
}
