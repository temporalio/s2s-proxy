package collect

import (
	"fmt"
	"iter"
)

type (
	staticBiMap[K, V comparable] struct {
		contents map[K]V
		inverse  *staticBiMap[V, K]
	}
	StaticBiMap[K, V comparable] interface {
		Get(key K) V
		GetExists(key K) (V, bool)
		Inverse() StaticBiMap[V, K]
	}
	ConflictError[T comparable] struct {
		isKey   bool
		newItem T
		// why existingItem? For some kinds of comparable, things with different content can be equal.
		// for those items, it's helpful to dump the value of the existing thing in addition
		existingItem T
	}
)

// NewStaticBiMap converts a set of disjoint pairs into two hash tables so that values can be easily looked up in either
// direction. Does not tolerate duplicate values in mappings and will error if they exist.
func NewStaticBiMap[K, V comparable](pairs iter.Seq2[K, V], expectedSize int) (StaticBiMap[K, V], error) {
	forward := &staticBiMap[K, V]{}
	backward := &staticBiMap[V, K]{}
	forward.contents = make(map[K]V, expectedSize)
	backward.contents = make(map[V]K, expectedSize)
	forward.inverse = backward
	backward.inverse = forward
	for key, val := range pairs {
		if value, ok := forward.contents[key]; ok {
			// Can't get the existing value without iterating the forward map! Rely on the backward map
			return nil, ConflictError[K]{true, key, backward.contents[value]}
		}
		forward.contents[key] = val
		if existing, ok := backward.contents[val]; ok {
			// Can't get the existing value without iterating the backward map! Rely on the forward map
			return nil, ConflictError[V]{true, val, forward.contents[existing]}
		}
		backward.contents[val] = key
	}
	return forward, nil
}

func (m *staticBiMap[K, V]) GetExists(key K) (V, bool) {
	val, exists := m.contents[key]
	return val, exists
}
func (m *staticBiMap[K, V]) Get(key K) V {
	return m.contents[key]
}
func (m *staticBiMap[K, V]) Inverse() StaticBiMap[V, K] {
	return m.inverse
}

func (e ConflictError[T]) Error() string {
	keyOrValue := "key"
	if !e.isKey {
		keyOrValue = "value"
	}
	return fmt.Sprintf("attempted to insert conflicting %s %v, which is identical to %v", keyOrValue, e.existingItem, e.newItem)
}
