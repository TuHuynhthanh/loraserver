package types

import uuid "github.com/satori/go.uuid"

// UUID wraps the UUID type as an API compatible type.
type UUID struct {
	uuid.UUID
}

// Marshal returns the UUID as bytes.
func (u UUID) Marshal() ([]byte, error) {
	return u.Bytes(), nil
}

// Unmarshal sets the UUID from bytes.
func (u *UUID) Unmarshal(data []byte) error {
	if len(data) != 0 {
		id, err := uuid.FromBytes(data)
		if err != nil {
			return err
		}
		u.UUID = id
	}

	return nil
}

// Size returns the size of the UUID object.
func (u UUID) Size() int {
	return len(u.Bytes())
}

// MarshalTo copies the UUID bytes to b.
func (u UUID) MarshalTo(b []byte) (int, error) {
	copy(b, u.Bytes())
	return u.Size(), nil
}
