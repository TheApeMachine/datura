package structure

import "reflect"

/*
isPowerOfTwo reports whether value is a positive integer that is exactly a power
of two. Ring slot stores index with mask (capacity-1), so capacities must satisfy
this predicate.
*/
func isPowerOfTwo(value int) bool {
	return value > 0 && (value&(value-1)) == 0
}

/*
nextPowerOfTwo returns the smallest power of two greater than or equal to value.
Used when Merge or Slice must allocate a buffer large enough for a combined or
detached segment without leaving the power-of-two indexing discipline.
*/
func nextPowerOfTwo(value int) int {
	if value < 1 {
		return 1
	}

	if isPowerOfTwo(value) {
		return value
	}

	power := 1

	for power < value {
		power <<= 1
	}

	return power
}

/*
isNilValue reports whether value is nil for pointer, map, channel, slice, interface,
or function types. For other types (including numeric zero) it always reports false.

Generic typed nil pointers do not satisfy any(value) == nil; reflect.IsNil handles
that case.
*/
func isNilValue[T any](value T) bool {
	if any(value) == nil {
		return true
	}

	reflected := reflect.ValueOf(value)

	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.Interface, reflect.Slice:
		return reflected.IsNil()
	}

	return false
}

/*
zeroValue is the Pop result for an empty queue ring. Callers that hold pointer
element types should use isNilValue to test emptiness rather than comparing to
zeroValue directly when T is not a pointer type.
*/
func zeroValue[T any]() T {
	var empty T

	return empty
}
