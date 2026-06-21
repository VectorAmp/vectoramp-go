package vectoramp

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// VectorID is a vector record identifier that preserves the caller's numeric or
// string type on the wire.
//
// The VectorAmp API accepts both string and integer vector ids and treats them
// as distinct: an integer id must be sent as a JSON number, not a quoted string,
// or the API rewrites it. Construct one with StringID or IntID, or rely on the
// convenience constructors that accept either:
//
//	Vector{ID: IntID(42), Values: vals}      // serializes "id": 42
//	Vector{ID: StringID("doc-1"), Values: …} // serializes "id": "doc-1"
//
// A zero VectorID marshals as null; AddTexts treats it as "generate an id".
type VectorID struct {
	value interface{}
}

// StringID returns a VectorID that serializes as a JSON string.
func StringID(id string) VectorID { return VectorID{value: id} }

// IntID returns a VectorID that serializes as a JSON number.
func IntID(id int64) VectorID { return VectorID{value: id} }

// NewVectorID coerces a string, integer, or nil into a VectorID, preserving the
// numeric/string distinction. Unsupported types produce an error.
func NewVectorID(id interface{}) (VectorID, error) {
	switch v := id.(type) {
	case nil:
		return VectorID{}, nil
	case VectorID:
		return v, nil
	case string:
		return VectorID{value: v}, nil
	case int:
		return VectorID{value: int64(v)}, nil
	case int32:
		return VectorID{value: int64(v)}, nil
	case int64:
		return VectorID{value: v}, nil
	case uint:
		return VectorID{value: int64(v)}, nil
	case uint32:
		return VectorID{value: int64(v)}, nil
	case uint64:
		return VectorID{value: int64(v)}, nil
	case json.Number:
		return VectorID{value: v}, nil
	default:
		return VectorID{}, fmt.Errorf("vectoramp: unsupported vector id type %T", id)
	}
}

// IsZero reports whether the VectorID is unset (no id provided).
func (id VectorID) IsZero() bool { return id.value == nil }

// String returns the textual form of the id regardless of underlying type.
func (id VectorID) String() string {
	switch v := id.value.(type) {
	case nil:
		return ""
	case string:
		return v
	case int64:
		return strconv.FormatInt(v, 10)
	case json.Number:
		return v.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

// Value returns the underlying id value (string, int64, json.Number, or nil).
func (id VectorID) Value() interface{} { return id.value }

// MarshalJSON serializes a VectorID, emitting a JSON number for integer ids and
// a JSON string for string ids. A zero VectorID marshals as null.
func (id VectorID) MarshalJSON() ([]byte, error) {
	if id.value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(id.value)
}

// UnmarshalJSON decodes a VectorID from a JSON number or string, preserving the
// type so round-tripped numeric ids stay numeric.
func (id *VectorID) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		id.value = nil
		return nil
	}
	if len(data) > 0 && data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		id.value = s
		return nil
	}
	num := json.Number(data)
	if i, err := num.Int64(); err == nil {
		id.value = i
		return nil
	}
	id.value = num
	return nil
}
