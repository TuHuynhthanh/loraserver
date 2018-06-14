package types

import "github.com/brocaar/lorawan"

// DevAddr wraps the DevAddr type as an API compatible type.
type DevAddr struct {
	lorawan.DevAddr
}

// Marshal returns the DevAddr as bytes.
func (d DevAddr) Marshal() ([]byte, error) {
	return d.DevAddr[:], nil
}

// Unmarshal sets the DevAddr from bytes.
func (d *DevAddr) Unmarshal(data []byte) error {
	if len(data) != 0 {
		copy(d.DevAddr[:], data)
	}

	return nil
}

// Size returns the size of the DevAddr object.
func (d DevAddr) Size() int {
	return len(d.DevAddr[:])
}

// MarshalTo copies the DevAddr bytes to b.
func (d DevAddr) MarshalTo(b []byte) (int, error) {
	copy(b, d.DevAddr[:])
	return d.Size(), nil
}
