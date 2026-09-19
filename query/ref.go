package query

import (
	"fmt"
	"reflect"
	"strings"
	"unsafe"
)

type ColRef struct {
	name string
}

func (c ColRef) String() string {
	return c.name
}

func (c ColRef) Str() string {
	return c.name
}

// Ref resolves the column name from the struct field's `db` tag
// by matching the memory offset between structPtr and fieldPtr.
func Ref[T, F any](structPtr *T, fieldPtr *F) ColRef {
	if structPtr == nil {
		panic("grain: Ref: struct pointer must not be nil")
	}
	if fieldPtr == nil {
		panic("grain: Ref: field pointer must not be nil")
	}

	val := reflect.ValueOf(structPtr).Elem()
	typ := val.Type()
	if typ.Kind() != reflect.Struct {
		panic(fmt.Sprintf("grain: Ref: expected pointer to struct, got pointer to %s", typ.Kind()))
	}

	sAddr := uintptr(unsafe.Pointer(structPtr))
	fAddr := uintptr(unsafe.Pointer(fieldPtr))
	targetOffset := fAddr - sAddr

	tag, ok := findTagByOffset(typ, 0, targetOffset)
	if !ok {
		panic(fmt.Sprintf("grain: Ref: pointer does not match any field in %s", typ.Name()))
	}
	return ColRef{name: tag}
}

// ref is a convenient alias for Ref.
func ref[T, F any](structPtr *T, fieldPtr *F) ColRef {
	return Ref(structPtr, fieldPtr)
}

func findTagByOffset(typ reflect.Type, baseOffset, targetOffset uintptr) (string, bool) {
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		fieldOffset := baseOffset + field.Offset
		if field.Anonymous && field.Type.Kind() == reflect.Struct {
			if tag, ok := findTagByOffset(field.Type, fieldOffset, targetOffset); ok {
				return tag, true
			}
		}
		if fieldOffset == targetOffset {
			tag := field.Tag.Get("db")
			if tag == "" {
				panic(fmt.Sprintf("grain: Ref: field %s.%s has no db tag", typ.Name(), field.Name))
			}
			if idx := strings.IndexByte(tag, ','); idx != -1 {
				tag = tag[:idx]
			}
			return tag, true
		}
	}
	return "", false
}
