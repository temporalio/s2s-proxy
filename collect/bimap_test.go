package collect

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBimap(t *testing.T) {
	bimap, err := NewStaticBiMap(func(yield func(string, string) bool) {
		for i := range 100 {
			if !yield(fmt.Sprintf("key%d", i), fmt.Sprintf("value%d", i)) {
				return
			}
		}
	}, 100)
	require.NoError(t, err)
	require.Equal(t, "value10", bimap.Get("key10"))
	require.Equal(t, "value99", bimap.Get("key99"))
	inverse := bimap.Inverse()
	require.Equal(t, "key10", inverse.Get("value10"))
	require.Equal(t, "key99", inverse.Get("value99"))
	require.Equal(t, bimap.Len(), inverse.Len())
	require.Equal(t, 100, bimap.Len())
	require.Equal(t, 100, len(bimap.AsMap()))
	require.Equal(t, "value58", bimap.AsMap()["key58"])
	require.Equal(t, "key58", inverse.AsMap()["value58"])
	val, found := bimap.GetExists("key100")
	require.False(t, found)
	require.Equal(t, "", val)
	require.Equal(t, "", bimap.Get("Not here"))
}

func TestNullBimap(t *testing.T) {
	var bimap staticBiMap[string, string]
	require.Equal(t, "", bimap.Get("key"))
	require.Nil(t, bimap.Inverse())
	require.Equal(t, 0, bimap.Len())
	require.Nil(t, bimap.AsMap())
	require.Equal(t, "", bimap.Inverse().Get("abcd"))
}

func TestBimapConflict(t *testing.T) {
	bimap, err := NewStaticBiMap(func(yield func(string, string) bool) {
		yield("key1", "value1")
		yield("key2", "value2")
		yield("key3", "value1")
	}, 10)
	require.Error(t, err)
	require.Nil(t, bimap)
	require.True(t, errors.As(err, &ConflictError[string]{}))
	require.Equal(t, "value1", err.(ConflictError[string]).existingItem)
	require.Equal(t, "value1", err.(ConflictError[string]).newItem)
	bimap, err = NewStaticBiMap(func(yield func(string, string) bool) {
		yield("key1", "value1")
		yield("key2", "value2")
		yield("key1", "value3")
	}, 10)
	require.Error(t, err)
	require.Nil(t, bimap)
	require.True(t, errors.As(err, &ConflictError[string]{}))
	require.Equal(t, "key1", err.(ConflictError[string]).existingItem)
	require.Equal(t, "key1", err.(ConflictError[string]).newItem)
}
