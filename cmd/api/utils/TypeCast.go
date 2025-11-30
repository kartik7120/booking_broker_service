package utils

func TypeCast[T any](value any) (T, bool) {
	var result T
	castedValue, ok := value.(T)
	if !ok {
		return result, false
	}
	return castedValue, true
}

// TypeCast is a utility function that attempts to cast a value to a specified type.
// It returns the casted value and a boolean indicating success or failure.
