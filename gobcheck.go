package fastcache

import (
	"encoding"
	"encoding/gob"
	"fmt"
	"reflect"
)

// checkGobTypes returns an error if gob cannot fully encode a key or value of
// type K or V. It reports the first problem found, checking K first. K and V
// are fields of the encoded entry, so they follow gob's field rules.
func checkGobTypes[K comparable, V any]() error {
	if err := checkGobField(reflect.TypeFor[K](), map[reflect.Type]bool{}); err != nil {
		return fmt.Errorf("%w (key type %v)", err, reflect.TypeFor[K]())
	}

	if err := checkGobField(reflect.TypeFor[V](), map[reflect.Type]bool{}); err != nil {
		return fmt.Errorf("%w (value type %v)", err, reflect.TypeFor[V]())
	}

	return nil
}

// checkGobField returns an error if gob cannot fully encode a struct field of
// type typ. gob never sends a chan or func field, even one whose type encodes
// itself.
func checkGobField(typ reflect.Type, seen map[reflect.Type]bool) error {
	if isChanOrFunc(typ) {
		return fmt.Errorf("%w: gob does not encode %v", errUnsupportedType, typ)
	}

	return checkGobType(typ, seen)
}

// checkGobType returns an error if gob cannot fully encode a value of type
// typ. Like encoding/gob, it skips unexported struct fields and rejects chan
// and func values that do not encode themselves. Interface types are not
// walked because their dynamic types are unknown here.
//
// A struct that embeds a self-encoding type such as time.Time inherits its
// GobEncode method, and gob then drops the struct's other fields. reflect
// cannot tell that promoted method from one the struct declares, so such
// types pass this check.
func checkGobType(typ reflect.Type, seen map[reflect.Type]bool) error {
	// Recursive types such as a linked list would otherwise never end.
	if seen[typ] {
		return nil
	}

	seen[typ] = true

	if encodesItself(typ) {
		return nil
	}

	switch typ.Kind() {
	case reflect.Chan, reflect.Func:
		return fmt.Errorf("%w: gob does not encode %v", errUnsupportedType, typ)
	case reflect.Pointer, reflect.Slice, reflect.Array:
		return checkGobType(typ.Elem(), seen)
	case reflect.Map:
		if err := checkGobType(typ.Key(), seen); err != nil {
			return err
		}

		return checkGobType(typ.Elem(), seen)
	case reflect.Struct:
		return checkGobStruct(typ, seen)
	default:
		return nil
	}
}

func checkGobStruct(typ reflect.Type, seen map[reflect.Type]bool) error {
	for i := range typ.NumField() {
		field := typ.Field(i)
		// Blank and zero-size fields hold no data, so gob skipping them loses
		// nothing. They serve as markers, such as `_ [0]func()` or a noCopy
		// guard.
		if field.Name == "_" || field.Type.Size() == 0 {
			continue
		}

		if !field.IsExported() {
			return fmt.Errorf("%w: gob does not encode unexported field %s of %v", errUnsupportedType, field.Name, typ)
		}

		if err := checkGobField(field.Type, seen); err != nil {
			return fmt.Errorf("%w (field %s of %v)", err, field.Name, typ)
		}
	}

	return nil
}

// encodesItself reports whether gob delegates encoding of typ to the type
// itself, through gob.GobEncoder or encoding.BinaryMarshaler on typ or *typ.
// checkGobType walks pointer types one level at a time, which covers gob's
// search through every pointer dereference.
func encodesItself(typ reflect.Type) bool {
	for _, cand := range []reflect.Type{typ, reflect.PointerTo(typ)} {
		if cand.Implements(reflect.TypeFor[gob.GobEncoder]()) ||
			cand.Implements(reflect.TypeFor[encoding.BinaryMarshaler]()) {
			return true
		}
	}

	return false
}

// isChanOrFunc reports whether typ is a chan or func after following
// pointers, as gob does when it decides which struct fields to send.
func isChanOrFunc(typ reflect.Type) bool {
	// Pointer types can form cycles, such as type P *P.
	followed := map[reflect.Type]bool{}
	for typ.Kind() == reflect.Pointer && !followed[typ] {
		followed[typ] = true
		typ = typ.Elem()
	}

	return typ.Kind() == reflect.Chan || typ.Kind() == reflect.Func
}
