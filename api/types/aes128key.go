package types

import "github.com/brocaar/lorawan"

// AES128Key wraps the AES128Key type as an API compatible type.
type AES128Key struct {
	lorawan.AES128Key
}

// Marshal returns the AES128Key as bytes.
func (k AES128Key) Marshal() ([]byte, error) {
	return k.AES128Key[:], nil
}

// Unmarshal sets the AES128Key from bytes.
func (k *AES128Key) Unmarshal(data []byte) error {
	if len(data) != 0 {
		copy(k.AES128Key[:], data)
	}
	return nil
}

// Size returns the size of the AES128Key object.
func (k AES128Key) Size() int {
	return len(k.AES128Key[:])
}

// MarshalTo copies the AES128Key bytes to by.
func (k AES128Key) MarshalTo(b []byte) (int, error) {
	copy(b, k.AES128Key[:])
	return k.Size(), nil
}
