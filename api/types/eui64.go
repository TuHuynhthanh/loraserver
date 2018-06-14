package types

import "github.com/brocaar/lorawan"

// EUI64 wraps the EUI64 type as an API compatible type.
type EUI64 struct {
	lorawan.EUI64
}

// Marshal returns the EUI64 as bytes.
func (e EUI64) Marshal() ([]byte, error) {
	return e.EUI64[:], nil
}

// Unmarshal sets the EUI64 from bytes.
func (e *EUI64) Unmarshal(data []byte) error {
	if len(data) != 0 {
		copy(e.EUI64[:], data)
	}
	return nil
}

// Size returns the size of the EUI64 object.
func (e EUI64) Size() int {
	return len(e.EUI64[:])
}

// MarshalTo copies the EUI64 bytes to b.
func (e EUI64) MarshalTo(b []byte) (int, error) {
	copy(b, e.EUI64[:])
	return e.Size(), nil
}
