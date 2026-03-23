// Package exec provides template execution support.
package exec

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/nikolalohinski/gonja/v2/logging"
	u "github.com/nikolalohinski/gonja/v2/utils"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type Value struct {
	Val  reflect.Value
	Safe bool // used to indicate whether a Value needs explicit escaping in the template
}

// AsValue converts any given Value to a gonja.Value.
// Usually being used within functions passed to a template
// through a Context or within filter functions.
//
// Example:
//
//	AsValue("my string")
func AsValue(i any) *Value {
	return &Value{
		Val: reflect.ValueOf(i),
	}
}

// AsSafeValue works like AsValue, but does not apply the 'escape' filter.
func AsSafeValue(i any) *Value {
	return &Value{
		Val:  reflect.ValueOf(i),
		Safe: true,
	}
}

func ValueError(err error) *Value {
	return &Value{Val: reflect.ValueOf(err)}
}

func (v *Value) getResolvedValue() reflect.Value {
	if v.Val.IsValid() && v.Val.Kind() == reflect.Pointer {
		return v.Val.Elem()
	}
	return v.Val
}

// IsString checks whether the underlying value is a string
func (v *Value) IsString() bool {
	return v.getResolvedValue().Kind() == reflect.String
}

// IsBool checks whether the underlying value is a bool
func (v *Value) IsBool() bool {
	return v.getResolvedValue().Kind() == reflect.Bool
}

// IsFloat checks whether the underlying value is a float
func (v *Value) IsFloat() bool {
	return v.getResolvedValue().Kind() == reflect.Float32 ||
		v.getResolvedValue().Kind() == reflect.Float64
}

// IsInteger checks whether the underlying value is an integer
func (v *Value) IsInteger() bool {
	kind := v.getResolvedValue().Kind()
	return kind == reflect.Int || kind == reflect.Int8 || kind == reflect.Int16 ||
		kind == reflect.Int32 || kind == reflect.Int64 || kind == reflect.Uint ||
		kind == reflect.Uint8 || kind == reflect.Uint16 || kind == reflect.Uint32 ||
		kind == reflect.Uint64
}

// IsNumber checks whether the underlying value is either an integer
// or a float.
func (v *Value) IsNumber() bool {
	return v.IsInteger() || v.IsFloat()
}

func (v *Value) IsCallable() bool {
	return v.getResolvedValue().Kind() == reflect.Func
}

func (v *Value) IsList() bool {
	kind := v.getResolvedValue().Kind()
	return kind == reflect.Array || kind == reflect.Slice
}

func (v *Value) IsDict() bool {
	resolved := v.getResolvedValue()
	return resolved.Kind() == reflect.Map || resolved.Kind() == reflect.Struct && resolved.Type() == TypeDict
}

func (v *Value) IsIterable() bool {
	return v.IsString() || v.IsList() || v.IsDict()
}

// IsNil checks whether the underlying value is nil
func (v *Value) IsNil() bool {
	return !v.getResolvedValue().IsValid()
}

func (v *Value) IsError() bool {
	if v.IsNil() || !v.getResolvedValue().CanInterface() {
		return false
	}
	_, ok := v.Interface().(error)
	return ok
}

func (v *Value) Error() string {
	if v.IsError() {
		return v.Interface().(error).Error()
	}
	return ""
}

func (v *Value) ToGoSimpleType(allowInterfaceKeys bool) any {
	switch {
	case v.IsError():
		return errors.New(v.Error())
	case v.IsNil():
		return nil
	case v.IsBool():
		return v.Bool()
	case v.IsFloat():
		return v.Float()
	case v.IsInteger():
		return v.Integer()
	case v.IsString():
		return v.String()
	case v.IsList():
		var err error
		list := make([]any, 0)
		v.Iterate(func(_, _ int, element, _ *Value) bool {
			casted := element.ToGoSimpleType(allowInterfaceKeys)
			var isError bool
			if err, isError = casted.(error); isError {
				return false
			}
			list = append(list, casted)
			return true
		}, func() {})
		if err != nil {
			return err
		}
		return list
	case v.IsIterable() && allowInterfaceKeys:
		var err error
		object := make(map[any]any)
		v.Iterate(func(_, _ int, key, value *Value) bool {
			var isError bool
			castedKey := key.ToGoSimpleType(allowInterfaceKeys)
			if err, isError = castedKey.(error); isError {
				return false
			}
			castedValue := value.ToGoSimpleType(allowInterfaceKeys)
			if err, isError = castedValue.(error); isError {
				return false
			}
			object[castedKey] = castedValue
			return true
		}, func() {})
		if err != nil {
			return err
		}
		return object
	case v.IsIterable() && !allowInterfaceKeys:
		var err error
		object := make(map[string]any)
		v.Iterate(func(_, _ int, key, value *Value) bool {
			var isError bool
			castedValue := value.ToGoSimpleType(allowInterfaceKeys)
			if err, isError = castedValue.(error); isError {
				return false
			}
			if !key.IsString() {
				err = fmt.Errorf("can not cast key %s to string", key.String())
				return false
			}
			object[key.String()] = castedValue
			return true
		}, func() {})
		if err != nil {
			return err
		}
		return object
	}
	if v.Val.CanInterface() {
		return v.Val.Interface()
	}
	return fmt.Errorf("cannot access unexported field")
}

// String returns a string for the underlying value. If this value is not
// of type string, gonja tries to convert it. Currently the following
// types for underlying values are supported:
//
//  1. string
//  2. int/uint (any size)
//  3. float (any precision)
//  4. bool
//  5. time.Time
//  6. String() will be called on the underlying value if provided
//
// nil values will lead to an empty string. For unsupported types, String will
// return to the type's name.
func (v *Value) String() string {
	if v.IsNil() {
		return ""
	}
	resolved := v.getResolvedValue()

	switch resolved.Kind() {
	case reflect.String:
		return resolved.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(resolved.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(resolved.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		formated := strconv.FormatFloat(resolved.Float(), 'f', 11, 64)
		if !strings.Contains(formated, ".") {
			formated = formated + "."
		}
		formated = strings.TrimRight(formated, "0")
		if formated[len(formated)-1] == '.' {
			formated += "0"
		}
		return formated
	case reflect.Bool:
		if v.Bool() {
			return "True"
		}
		return "False"
	case reflect.Struct:
		if t, ok := v.Interface().(fmt.Stringer); ok {
			return t.String()
		}
	case reflect.Slice, reflect.Array:
		var out strings.Builder
		// Special case for []byte
		if resolved.Type().Elem().Kind() == reflect.Uint8 {
			out.WriteString("b'")
			for _, b := range resolved.Bytes() {
				out.WriteByte(b)
			}
			out.WriteString("'")
			return out.String()
		}
		out.WriteByte('[')
		length := v.Len()
		for i := range length {
			if i > 0 {
				out.WriteString(", ")
			}
			item, ok := v.Index(i).Val.Interface().(*Value)
			if !ok {
				item = ToValue(v.Index(i).Val)
			}
			if item.IsString() {
				out.WriteString(fmt.Sprintf(`'%s'`, item.String()))
			} else {
				out.WriteString(item.String())
			}
		}
		out.WriteByte(']')
		return out.String()
	case reflect.Map:
		pairs := []string{}
		for _, key := range resolved.MapKeys() {
			keyLabel := key.String()
			if key.Kind() == reflect.String {
				keyLabel = fmt.Sprintf(`'%s'`, keyLabel)
			}

			value := resolved.MapIndex(key)
			// Check whether this is an interface and resolve it where required
			for value.Kind() == reflect.Interface {
				value = reflect.ValueOf(value.Interface())
			}
			valueLabel := ToValue(value).String()
			if value.Kind() == reflect.String {
				valueLabel = fmt.Sprintf(`'%s'`, valueLabel)
			}
			pair := fmt.Sprintf(`%s: %s`, keyLabel, valueLabel)
			pairs = append(pairs, pair)
		}
		sort.Strings(pairs)
		return fmt.Sprintf("{%s}", strings.Join(pairs, ", "))
	}

	if logging.Enabled() {
		log.Errorf("Value.String() not implemented for type: %s\n", resolved.Kind().String())
	}
	return resolved.String()
}

// Escaped returns the escaped version of String()
func (v *Value) Escaped() string {
	return u.Escape(v.String())
}

// Integer returns the underlying value as an integer (converts the underlying
// value, if necessary). If it's not possible to convert the underlying value,
// it will return 0.
func (v *Value) Integer() int {
	switch v.getResolvedValue().Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return int(v.getResolvedValue().Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int(v.getResolvedValue().Uint())
	case reflect.Float32, reflect.Float64:
		return int(v.getResolvedValue().Float())
	case reflect.String:
		// Try to convert from string to int (base 10)
		f, err := strconv.ParseFloat(v.getResolvedValue().String(), 64)
		if err != nil {
			return 0
		}
		return int(f)
	default:
		if logging.Enabled() {
			log.Errorf("Value.Integer() not available for type: %s\n", v.getResolvedValue().Kind().String())
		}
		return 0
	}
}

// Float returns the underlying value as a float (converting the underlying
// value, if necessary). If it's not possible to convert the underlying value,
// it will return 0.0.
func (v *Value) Float() float64 {
	switch v.getResolvedValue().Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(v.getResolvedValue().Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(v.getResolvedValue().Uint())
	case reflect.Float32, reflect.Float64:
		return v.getResolvedValue().Float()
	case reflect.String:
		// Try to convert from string to float64 (base 10)
		f, err := strconv.ParseFloat(v.getResolvedValue().String(), 64)
		if err != nil {
			return 0.0
		}
		return f
	default:
		if logging.Enabled() {
			log.Errorf("Value.Float() not available for type: %s\n", v.getResolvedValue().Kind().String())
		}
		return 0.0
	}
}

// Bool returns the underlying value as a bool. If the value is not a bool, false
// will always be returned. If you're looking for true/false-evaluation of the
// underlying value, have a look at the IsTrue() function.
func (v *Value) Bool() bool {
	switch v.getResolvedValue().Kind() {
	case reflect.Bool:
		return v.getResolvedValue().Bool()
	default:
		if logging.Enabled() {
			log.Errorf("Value.Bool() not available for type: %s\n", v.getResolvedValue().Kind().String())
		}
		return false
	}
}

// IsTrue tries to evaluate the underlying value the Pythonic-way by returning
// true in one the following cases:
//
//   - int != 0
//   - uint != 0
//   - float != 0.0
//   - len(array/chan/map/slice/string) > 0
//   - bool == true
//   - underlying value is a struct
//
// In any other case, IsTrue returns false.
func (v *Value) IsTrue() bool {
	if v.IsNil() || v.IsError() {
		return false
	}
	switch v.getResolvedValue().Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.getResolvedValue().Int() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.getResolvedValue().Uint() != 0
	case reflect.Float32, reflect.Float64:
		return v.getResolvedValue().Float() != 0
	case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice, reflect.String:
		return v.getResolvedValue().Len() > 0
	case reflect.Bool:
		return v.getResolvedValue().Bool()
	case reflect.Struct:
		return true // struct instance is always true
	default:
		if logging.Enabled() {
			log.Errorf("Value.IsTrue() not available for type: %s\n", v.getResolvedValue().Kind().String())
		}
		return false
	}
}

// Negate tries to negate the underlying value. It's mainly used for
// the NOT-operator and in conjunction with a call to
// return_value.IsTrue() afterwards.
//
// Example:
//
//	AsValue(1).Negate().IsTrue() == false
func (v *Value) Negate() *Value {
	if v.IsNil() || v.IsError() {
		return AsValue(true)
	}
	switch v.getResolvedValue().Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if v.Integer() != 0 {
			return AsValue(0)
		}
		return AsValue(1)
	case reflect.Float32, reflect.Float64:
		if v.Float() != 0.0 {
			return AsValue(float64(0.0))
		}
		return AsValue(float64(1.1))
	case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice, reflect.String:
		return AsValue(v.getResolvedValue().Len() == 0)
	case reflect.Bool:
		return AsValue(!v.getResolvedValue().Bool())
	case reflect.Struct:
		return AsValue(false)
	default:
		if logging.Enabled() {
			log.Errorf("Value.Negate() not available for type: %s\n", v.getResolvedValue().Kind().String())
		}
		return AsValue(true)
	}
}

// Len returns the length for an array, chan, map, slice or string.
// Otherwise it will return 0.
func (v *Value) Len() int {
	switch v.getResolvedValue().Kind() {
	case reflect.Array, reflect.Chan, reflect.Slice:
		return v.getResolvedValue().Len()
	case reflect.Map:
		return len(v.Keys())
	case reflect.String:
		runes := []rune(v.getResolvedValue().String())
		return len(runes)
	case reflect.Struct:
		if v.getResolvedValue().Type() == TypeDict {
			return len(v.Keys())
		}
		fallthrough
	default:
		if logging.Enabled() {
			log.Errorf("Value.Len() not available for type: %s\n", v.getResolvedValue().Kind().String())
		}
		return 0
	}
}

// Slice slices an array, slice or string. Otherwise it will
// return an empty []int.
func (v *Value) Slice(i, j int) *Value {
	switch v.getResolvedValue().Kind() {
	case reflect.Array, reflect.Slice:
		return AsValue(v.getResolvedValue().Slice(i, j).Interface())
	case reflect.String:
		runes := []rune(v.getResolvedValue().String())
		return AsValue(string(runes[i:j]))
	default:
		if logging.Enabled() {
			log.Errorf("Value.Slice() not available for type: %s\n", v.getResolvedValue().Kind().String())
		}
		return AsValue([]int{})
	}
}

// Index gets the i-th item of an array, slice or string. Otherwise
// it will return nil.
func (v *Value) Index(i int) *Value {
	switch v.getResolvedValue().Kind() {
	case reflect.Array, reflect.Slice:
		if i >= v.Len() {
			return AsValue(nil)
		}
		return AsValue(v.getResolvedValue().Index(i).Interface())
	case reflect.String:
		s := v.getResolvedValue().String()
		runes := []rune(s)
		if i < len(runes) {
			return AsValue(string(runes[i]))
		}
		return AsValue("")
	default:
		if logging.Enabled() {
			log.Errorf("Value.Slice() not available for type: %s\n", v.getResolvedValue().Kind().String())
		}
		return AsValue([]int{})
	}
}

// Contains checks whether the underlying value (which must be of type struct, map,
// string, array or slice) contains of Value (e.g. to check
// whether a struct contains a specific field or a map contains a specific key).
//
// Example:
//
//	AsValue("Hello, World!").Contains(AsValue("World")) == true
func (v *Value) Contains(other *Value) bool {
	resolved := v.getResolvedValue()
	switch resolved.Kind() {
	case reflect.Struct:
		if dict, ok := resolved.Interface().(Dict); ok {
			return dict.Keys().Contains(other)
		}
		fieldValue := resolved.FieldByName(other.String())
		return fieldValue.IsValid()
	case reflect.Map:
		var mapValue reflect.Value
		switch other.Interface().(type) {
		case int:
			mapValue = resolved.MapIndex(other.getResolvedValue())
		case string:
			mapValue = resolved.MapIndex(other.getResolvedValue())
		default:
			if logging.Enabled() {
				log.Errorf("Value.Contains() does not support lookup type '%s'\n", other.getResolvedValue().Kind().String())
			}
			return false
		}

		return mapValue.IsValid()
	case reflect.String:
		return strings.Contains(resolved.String(), other.String())

	case reflect.Slice, reflect.Array:
		if vl, ok := resolved.Interface().(ValuesList); ok {
			return vl.Contains(other)
		}
		for i := 0; i < resolved.Len(); i++ {
			item := resolved.Index(i)
			if other.Interface() == item.Interface() {
				return true
			}
		}
		return false

	default:
		if logging.Enabled() {
			log.Errorf("Value.Contains() not available for type: %s\n", resolved.Kind().String())
		}
		return false
	}
}

// CanSlice checks whether the underlying value is of type array, slice or string.
// You normally would use CanSlice() before using the Slice() operation.
func (v *Value) CanSlice() bool {
	switch v.getResolvedValue().Kind() {
	case reflect.Array, reflect.Slice, reflect.String:
		return true
	}
	return false
}

// Iterate iterates over a map, array, slice or a string. It calls the
// function's first argument for every value with the following arguments:
//
//	idx      current 0-index
//	count    total amount of items
//	key      *Value for the key or item
//	value    *Value (only for maps, the respective value for a specific key)
//
// If the underlying value has no items or is not one of the types above,
// the empty function (Iterate's second argument) will be called.
func (v *Value) Iterate(fn func(idx, count int, key, value *Value) bool, empty func()) {
	v.IterateOrder(fn, empty, false, false, false)
}

// IterateOrder behaves like Value.Iterate, but can iterate through an array/slice/string in reverse. Does
// not affect the iteration through a map because maps don't have any particular order.
// However, you can force an order using the `sorted` keyword (and even use `reversed sorted`).
func (v *Value) IterateOrder(fn func(idx, count int, key, value *Value) bool, empty func(), reverse bool, sorted bool, caseSensitive bool) {
	resolved := v.getResolvedValue()
	switch resolved.Kind() {
	case reflect.Map:
		keys := v.Keys()
		if sorted {
			if reverse {
				if !caseSensitive {
					sort.Sort(sort.Reverse(CaseInsensitive(keys)))
				} else {
					sort.Sort(sort.Reverse(keys))
				}
			} else {
				if !caseSensitive {
					sort.Sort(CaseInsensitive(keys))
				} else {
					sort.Sort(keys)
				}
			}
		}
		keyLen := len(keys)
		for idx, key := range keys {
			value, _ := v.GetItem(key.Interface())
			if !fn(idx, keyLen, key, value) {
				return
			}
		}
		if keyLen == 0 {
			empty()
		}
		return // done
	case reflect.Array, reflect.Slice:
		var items ValuesList

		itemCount := resolved.Len()
		for i := range itemCount {
			// value := resolved.Index(i)

			items = append(items, ToValue(resolved.Index(i)))
		}

		if sorted && itemCount > 0 {
			if reverse {
				if !caseSensitive && items[0].IsString() {
					sort.Slice(items, func(i, j int) bool {
						return strings.ToLower(items[i].String()) > strings.ToLower(items[j].String())
					})
				} else {
					sort.Sort(sort.Reverse(items))
				}
			} else {
				if !caseSensitive && items[0].IsString() {
					sort.Slice(items, func(i, j int) bool {
						return strings.ToLower(items[i].String()) < strings.ToLower(items[j].String())
					})
				} else {
					sort.Sort(items)
				}
			}
		} else if itemCount > 2 {
			if reverse {
				for i := 0; i < itemCount/2; i++ {
					items[i], items[itemCount-1-i] = items[itemCount-1-i], items[i]
				}
			}
		}

		if len(items) > 0 {
			for idx, item := range items {
				if !fn(idx, itemCount, item, nil) {
					return
				}
			}
		} else {
			empty()
		}
		return // done
	case reflect.String:
		if sorted {
			r := []rune(resolved.String())
			if caseSensitive {
				sort.Sort(sortRunes(r))
			} else {
				sort.Sort(CaseInsensitive(sortRunes(r)))
			}
			resolved = reflect.ValueOf(string(r))
		}

		// TODO(flosch): Not utf8-compatible (utf8-decoding necessary)
		charCount := resolved.Len()
		if charCount > 0 {
			if reverse {
				for i := charCount - 1; i >= 0; i-- {
					if !fn(i, charCount, &Value{Val: resolved.Slice(i, i+1)}, nil) {
						return
					}
				}
			} else {
				for i := range charCount {
					if !fn(i, charCount, &Value{Val: resolved.Slice(i, i+1)}, nil) {
						return
					}
				}
			}
		} else {
			empty()
		}
		return // done
	case reflect.Chan:
		items := []reflect.Value{}
		for {
			value, ok := resolved.Recv()
			if !ok {
				break
			}
			items = append(items, value)
		}
		count := len(items)
		if count > 0 {
			for idx, value := range items {
				fn(idx, count, &Value{Val: value}, nil)
			}
		} else {
			empty()
		}
		return
	case reflect.Struct:
		if resolved.Type() != TypeDict {
			if logging.Enabled() {
				log.Errorf("Value.Iterate() not available for type: %s\n", resolved.Kind().String())
			}
		}
		dict := resolved.Interface().(Dict)
		keys := dict.Keys()
		length := len(dict.Pairs)
		if sorted {
			if reverse {
				if !caseSensitive {
					sort.Sort(sort.Reverse(CaseInsensitive(keys)))
				} else {
					sort.Sort(sort.Reverse(keys))
				}
			} else {
				if !caseSensitive {
					sort.Sort(CaseInsensitive(keys))
				} else {
					sort.Sort(keys)
				}
			}
		}
		if len(keys) > 0 {
			for idx, key := range keys {
				if !fn(idx, length, key, dict.Get(key)) {
					return
				}
			}
		} else {
			empty()
		}

	default:
		if logging.Enabled() {
			log.Errorf("Value.Iterate() not available for type: %s\n", resolved.Kind().String())
		}
	}
	empty()
}

// Interface returns the underlying value.
func (v *Value) Interface() any {
	if v.Val.IsValid() && v.Val.CanInterface() {
		return v.Val.Interface()
	}
	return nil
}

// EqualValueTo checks whether two values are equal.
func (v *Value) EqualValueTo(other *Value) bool {
	// comparison of uint with int fails using .Interface()-comparison (see issue #64)
	if v.IsInteger() && other.IsInteger() {
		return v.Integer() == other.Integer()
	}
	// comparison of float with int also fails using .Interface()-comparison
	if v.IsNumber() && other.IsNumber() {
		return v.Float() == other.Float()
	}
	return v.Interface() == other.Interface()
}

func (v *Value) Keys() ValuesList {
	keys := ValuesList{}
	if v.IsNil() {
		return keys
	}
	resolved := v.getResolvedValue()
	if resolved.Type() == TypeDict {
		for _, pair := range resolved.Interface().(Dict).Pairs {
			keys = append(keys, pair.Key)
		}
		return keys
	} else if resolved.Kind() != reflect.Map {
		return keys
	}
	for _, key := range resolved.MapKeys() {
		keys = append(keys, &Value{Val: key})
	}
	sort.Sort(CaseInsensitive(keys))
	return keys
}

func (v *Value) Items() []*Pair {
	out := []*Pair{}
	resolved := v.getResolvedValue()
	if resolved.Kind() != reflect.Map {
		return out
	}
	iter := resolved.MapRange()
	for iter.Next() {
		out = append(out, &Pair{
			Key:   &Value{Val: iter.Key()},
			Value: &Value{Val: iter.Value()},
		})
	}
	return out
}

func ToValue(data any) *Value {
	var isSafe bool
	// if data == nil {
	// 	return AsValue(nil), nil
	// }
	value, ok := data.(*Value)
	if ok {
		return value
	}

	val, ok := data.(reflect.Value)
	if !ok {
		val = reflect.ValueOf(data) // Get the initial value
	}

	if !val.IsValid() {
		// Value is not valid (anymore)
		return AsValue(nil)
	}

	if val.Type() == reflect.TypeFor[reflect.Value]() {
		val = val.Interface().(reflect.Value)
	} else if val.Type() == reflect.TypeFor[*reflect.Value]() {
		val = *(val.Interface().(*reflect.Value))
	}

	if !val.IsValid() {
		// Value is not valid (anymore)
		return AsValue(nil)
	}

	// Check whether this is an interface and resolve it where required
	for val.Kind() == reflect.Interface {
		val = reflect.ValueOf(val.Interface())
	}

	if !val.IsValid() {
		// Value is not valid (anymore)
		return AsValue(nil)
	}

	// If val is a reflect.ValueOf(gonja.Value), then unpack it
	// Happens in function calls (as a return value) or by injecting
	// into the execution context (e.g. in a for-loop)
	if val.Type() == typeOfValuePtr {
		tmpValue := val.Interface().(*Value)
		val = tmpValue.Val
		isSafe = tmpValue.Safe
	}

	if !val.IsValid() {
		// Value is not valid (e.g. nil value)
		return AsValue(nil)
	}
	return &Value{Val: val, Safe: isSafe}
}

func (v *Value) GetAttribute(name string) (*Value, bool) {
	if v.IsNil() {
		return AsValue(errors.New(`Can't use getattr on None`)), false
	}
	var val reflect.Value
	val = v.Val.MethodByName(name)
	if val.IsValid() {
		return ToValue(val), true
	}
	if v.Val.Kind() == reflect.Pointer {
		val = v.Val.Elem()
		if !val.IsValid() {
			// Value is not valid (anymore)
			return AsValue(nil), false
		}
	} else {
		val = v.Val
	}

	if val.Kind() == reflect.Struct {
		field := val.FieldByName(name)
		if field.IsValid() {
			return ToValue(field), true
		}
	}

	return AsValue(nil), false // Attr not found
}

func (v *Value) GetItem(key any) (*Value, bool) {
	if v.IsNil() {
		return AsValue(errors.New(`Can't use Getitem on None`)), false
	}
	var val reflect.Value
	if v.Val.Kind() == reflect.Pointer {
		val = v.Val.Elem()
		if !val.IsValid() {
			// Value is not valid (anymore)
			return AsValue(nil), false
		}
	} else {
		val = v.Val
	}

	switch t := key.(type) {
	case string:
		if val.Kind() == reflect.Map {
			atKey := val.MapIndex(reflect.ValueOf(t))
			if atKey.IsValid() {
				return ToValue(atKey), true
			}
		} else if val.Kind() == reflect.Struct && val.Type() == TypeDict {
			for _, pair := range val.Interface().(Dict).Pairs {
				if pair.Key.String() == t {
					return pair.Value, true
				}
			}
		}

	case int:
		switch val.Kind() {
		case reflect.String:
			if t >= 0 && val.Len() > t {
				return ToValue(string(val.String()[t])), true
			} else if t < 0 && val.Len() > -t {
				return ToValue(string(val.String()[val.Len()+t])), true
			}
		case reflect.Array, reflect.Slice:
			if t >= 0 && val.Len() > t {
				atIndex := val.Index(t)
				if atIndex.IsValid() {
					return ToValue(atIndex), true
				}
			} else if t < 0 && val.Len() > -t {
				atIndex := val.Index(val.Len() + t)
				if atIndex.IsValid() {
					return ToValue(atIndex), true
				}
			}
		default:
			return AsValue(errors.Errorf("Can't access an index on type %s (variable %s)", val.Kind().String(), v)), false
		}
	default:
		return AsValue(nil), false
	}

	return AsValue(nil), false // Item not found
}

func (v *Value) Get(key string) (*Value, bool) {
	value, found := v.GetAttribute(key)
	if !found {
		value, found = v.GetItem(key)
	}
	return value, found
}

func (v *Value) Set(key *Value, value any) error {
	if v.IsNil() {
		return errors.New(`Can't set attribute or item on None`)
	}
	val := v.Val
	for val.Kind() == reflect.Pointer {
		val = val.Elem()
		if !val.IsValid() {
			// Value is not valid (anymore)
			return errors.Errorf(`Invalid value "%s"`, val)
		}
	}

	switch val.Kind() {
	case reflect.Struct:
		if !key.IsString() {
			return errors.Errorf(`Can't write non-string field "%s" to struct: %s`, key.String(), value)
		}
		field := val.FieldByName(key.String())
		if field.IsValid() && field.CanSet() {
			field.Set(reflect.ValueOf(value))
		} else {
			return errors.Errorf(`Can't write field "%s"`, key.String())
		}
	case reflect.Map:
		val.SetMapIndex(key.Val, reflect.ValueOf(value))
	default:
		return errors.Errorf(`Unknown type "%s", can't set value on "%s"`, val.Kind(), key.String())
	}

	return nil
}

type ValuesList []*Value

func (vl ValuesList) Len() int {
	return len(vl)
}

func (vl ValuesList) Less(i, j int) bool {
	vi := vl[i]
	vj := vl[j]
	switch {
	case vi.IsInteger() && vj.IsInteger():
		return vi.Integer() < vj.Integer()
	case vi.IsFloat() && vj.IsFloat():
		return vi.Float() < vj.Float()
	default:
		return vi.String() < vj.String()
	}
}

func (vl ValuesList) Swap(i, j int) {
	vl[i], vl[j] = vl[j], vl[i]
}

func (vl ValuesList) String() string {
	var out strings.Builder
	out.WriteByte('[')
	for idx, key := range vl {
		if idx > 0 {
			out.WriteString(", ")
		}
		if key.IsString() {
			out.WriteString("'")
		}
		out.WriteString(key.String())
		if key.IsString() {
			out.WriteString("'")
		}
	}
	out.WriteByte(']')
	return out.String()
}

func (vl ValuesList) Contains(value *Value) bool {
	return slices.ContainsFunc(vl, value.EqualValueTo)
}

type Pair struct {
	Key   *Value
	Value *Value
}

func (p *Pair) String() string {
	var key, value string
	if p.Key.IsString() {
		key = fmt.Sprintf(`'%s'`, p.Key.String())
	} else {
		key = p.Key.String()
	}
	if p.Value.IsString() {
		value = fmt.Sprintf(`'%s'`, p.Value.String())
	} else {
		value = p.Value.String()
	}
	return fmt.Sprintf(`%s: %s`, key, value)
}

type Dict struct {
	Pairs []*Pair
}

func NewDict() *Dict {
	return &Dict{Pairs: []*Pair{}}
}

func (d *Dict) String() string {
	pairs := []string{}
	for _, pair := range d.Pairs {
		pairs = append(pairs, pair.String())
	}
	return fmt.Sprintf(`{%s}`, strings.Join(pairs, ", "))
}

func (d *Dict) Keys() ValuesList {
	keys := ValuesList{}
	for _, pair := range d.Pairs {
		keys = append(keys, pair.Key)
	}
	return keys
}

func (d *Dict) Get(key *Value) *Value {
	for _, pair := range d.Pairs {
		if pair.Key.EqualValueTo(key) {
			return pair.Value
		}
	}
	return AsValue(nil)
}

var TypeDict = reflect.TypeFor[Dict]()

type sortRunes []rune

func (s sortRunes) Less(i, j int) bool {
	return s[i] < s[j]
}

func (s sortRunes) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s sortRunes) Len() int {
	return len(s)
}

type caseInsensitiveSortedRunes struct {
	sortRunes
}

func (ci caseInsensitiveSortedRunes) Less(i, j int) bool {
	return strings.ToLower(string(ci.sortRunes[i])) < strings.ToLower(string(ci.sortRunes[j]))
}

type caseInsensitiveValueList struct {
	ValuesList
}

func (ci caseInsensitiveValueList) Less(i, j int) bool {
	vi := ci.ValuesList[i]
	vj := ci.ValuesList[j]
	switch {
	case vi.IsInteger() && vj.IsInteger():
		return vi.Integer() < vj.Integer()
	case vi.IsFloat() && vj.IsFloat():
		return vi.Float() < vj.Float()
	default:
		return strings.ToLower(vi.String()) < strings.ToLower(vj.String())
	}
}

// CaseInsensitive returns the data sorted in a case insensitive way (if string).
func CaseInsensitive(data sort.Interface) sort.Interface {
	if vl, ok := data.(ValuesList); ok {
		return &caseInsensitiveValueList{vl}
	} else if sr, ok := data.(sortRunes); ok {
		return &caseInsensitiveSortedRunes{sr}
	}
	return data
}
