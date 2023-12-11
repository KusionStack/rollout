package utils

// Filter filter items which meet the predicate
func Filter[T any](items []T, predicate func(*T) bool) []T {
	var hits []T
	for i := range items {
		e := items[i]
		if predicate(&e) {
			hits = append(hits, e)
		}
	}
	return hits
}

// Find find first item which meet the predicate
func Find[T any](items []T, predicate func(*T) bool) (*T, bool) {
	for i := range items {
		e := items[i]
		if predicate(&e) {
			return &e, true
		}
	}
	return nil, false
}

// Any detect if item exist in the items
func Any[T any](items []T, predicate func(*T) bool) bool {
	for i := range items {
		e := items[i]
		if predicate(&e) {
			return true
		}
	}
	return false
}
