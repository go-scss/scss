// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

// This file implements the nested/deep map operations of the sass:map module:
// map.get/set/merge/has-key with a key path, plus map.deep-merge and
// map.deep-remove. dart-sass treats a non-map value encountered along a write
// path as an empty map (it is replaced), and leaves a map unchanged when a read
// or removal path cannot be followed.

// cloneMap returns a shallow copy of m with independent key/value slices.
func cloneMap(m *Map) *Map {
	return &Map{Keys: append([]Value(nil), m.Keys...), Values: append([]Value(nil), m.Values...)}
}

// mapRemoveKey returns a copy of m with key removed (if present).
func mapRemoveKey(m *Map, key Value) *Map {
	out := &Map{}
	for i := range m.Keys {
		if !m.Keys[i].equals(key) {
			out.set(m.Keys[i], m.Values[i])
		}
	}
	return out
}

// asMapOrEmpty coerces v to a map, treating any non-map (including a missing
// value) as an empty map — the behaviour dart-sass uses when writing through a
// key path whose intermediate value is not a map.
func asMapOrEmpty(v Value, ok bool) *Map {
	if ok {
		if m, isMap := v.(*Map); isMap {
			return m
		}
	}
	return &Map{}
}

// mapSetPath returns m with value written at the given non-empty key path,
// creating intermediate maps as needed.
func mapSetPath(m *Map, keys []Value, value Value) *Map {
	out := cloneMap(m)
	if len(keys) == 1 {
		out.set(keys[0], value)
		return out
	}
	child, ok := out.get(keys[0])
	out.set(keys[0], mapSetPath(asMapOrEmpty(child, ok), keys[1:], value))
	return out
}

// mapMergeShallow merges m2 onto a copy of m1 (m2's entries win).
func mapMergeShallow(m1, m2 *Map) *Map {
	out := cloneMap(m1)
	for i := range m2.Keys {
		out.set(m2.Keys[i], m2.Values[i])
	}
	return out
}

// mapMergePath merges m2 into the submap of m1 reached by keys, creating
// intermediate maps as needed. An empty path is a plain shallow merge.
func mapMergePath(m1 *Map, keys []Value, m2 *Map) *Map {
	if len(keys) == 0 {
		return mapMergeShallow(m1, m2)
	}
	out := cloneMap(m1)
	child, ok := out.get(keys[0])
	out.set(keys[0], mapMergePath(asMapOrEmpty(child, ok), keys[1:], m2))
	return out
}

// mapish coerces a value to a map for deep operations, treating the empty list
// () as the empty map (as Sass does). It reports whether v is map-like.
func mapish(v Value) (*Map, bool) {
	switch x := v.(type) {
	case *Map:
		return x, true
	case *List:
		if len(x.Elements) == 0 {
			return &Map{}, true
		}
	}
	return nil, false
}

// mapDeepMerge recursively merges m2 into m1: where both hold a map at the same
// key, those maps are themselves deep-merged; otherwise m2's value wins.
func mapDeepMerge(m1, m2 *Map) *Map {
	out := cloneMap(m1)
	for i := range m2.Keys {
		k, v2 := m2.Keys[i], m2.Values[i]
		if existing, ok := out.get(k); ok {
			if em, eok := mapish(existing); eok {
				if v2m, v2ok := mapish(v2); v2ok {
					out.set(k, mapDeepMerge(em, v2m))
					continue
				}
			}
		}
		out.set(k, v2)
	}
	return out
}

// mapDeepRemove returns m with the entry at the given non-empty key path
// removed. If the path cannot be followed, m is returned unchanged.
func mapDeepRemove(m *Map, keys []Value) *Map {
	if len(keys) == 1 {
		return mapRemoveKey(m, keys[0])
	}
	child, ok := m.get(keys[0])
	cm, isMap := child.(*Map)
	if !ok || !isMap {
		return m
	}
	out := cloneMap(m)
	out.set(keys[0], mapDeepRemove(cm, keys[1:]))
	return out
}

// mapHasKeyPath reports whether the key path exists in m, following only
// through maps.
func mapHasKeyPath(m *Map, keys []Value) bool {
	cur := m
	for i := 0; i < len(keys)-1; i++ {
		child, ok := cur.get(keys[i])
		if !ok {
			return false
		}
		cm, isMap := child.(*Map)
		if !isMap {
			return false
		}
		cur = cm
	}
	_, ok := cur.get(keys[len(keys)-1])
	return ok
}

// mapKeyArgs returns the map (argument 0) plus the list of key arguments,
// honouring both the positional rest form and the $key named-argument
// convention used by map.has-key/remove/deep-remove.
func mapKeyArgs(ci *callInfo) (*Map, []Value) {
	m := asMapVal(ci, ci.require(0, "map"))
	var keys []Value
	if len(ci.positional) >= 2 {
		keys = append(keys, ci.positional[1:]...)
	} else if k, ok := ci.named["key"]; ok {
		keys = append(keys, k)
	}
	return m, keys
}
