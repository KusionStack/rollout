package utils

// Filter filter items which meet the predicate
func Filter[T any](items []T, predicate func(*T) bool) []T {
	var hits []T
	if len(items) > 0 {
		for _, e := range items {
			if predicate(&e) {
				hits = append(hits, e)
			}
		}
	}
	return hits
}

// Find find first item which meet the predicate
func Find[T any](items []T, predicate func(*T) bool) (T, bool) {
	if len(items) > 0 {
		for _, e := range items {
			if predicate(&e) {
				return e, true
			}
		}
	}
	var zero T
	return zero, false
}

// Any detect if item exist in the items
func Any[T any](items []T, predicate func(*T) bool) bool {
	if len(items) > 0 {
		for _, e := range items {
			if predicate(&e) {
				return true
			}
		}
	}
	return false
}
