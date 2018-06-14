package types

import (
	"testing"

	"github.com/brocaar/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

func TestEUI64(t *testing.T) {
	Convey("Testing an EUI64", t, func() {
		e := EUI64{EUI64: lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}}
		b, err := e.Marshal()
		So(err, ShouldBeNil)
		So(b, ShouldResemble, e.EUI64[:])

		So(e.Size(), ShouldEqual, 8)

		b2 := make([]byte, e.Size())
		i, err := e.MarshalTo(b2)
		So(err, ShouldBeNil)
		So(i, ShouldEqual, e.Size())
		So(b2, ShouldResemble, b)

		var e2 EUI64
		So(e2.Unmarshal(b), ShouldBeNil)
		So(e2, ShouldResemble, e)
	})
}
