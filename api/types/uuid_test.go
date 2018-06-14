package types

import (
	"testing"

	uuid "github.com/satori/go.uuid"
	. "github.com/smartystreets/goconvey/convey"
)

func TestUUID(t *testing.T) {
	Convey("Testing an UUID", t, func() {
		id := uuid.NewV4()

		u := UUID{UUID: id}
		b, err := u.Marshal()
		So(err, ShouldBeNil)
		So(b, ShouldResemble, id.Bytes())

		So(u.Size(), ShouldEqual, len(b))

		b2 := make([]byte, len(b))
		i, err := u.MarshalTo(b2)
		So(err, ShouldBeNil)
		So(i, ShouldEqual, u.Size())
		So(b2, ShouldResemble, b)

		var u2 UUID
		So(u2.Unmarshal(b), ShouldBeNil)
		So(u2, ShouldResemble, u)
	})
}
