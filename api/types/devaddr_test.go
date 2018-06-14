package types

import (
	"testing"

	"github.com/brocaar/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

func TestDevAddr(t *testing.T) {
	Convey("Testing a DevAddr", t, func() {
		d := DevAddr{DevAddr: lorawan.DevAddr{1, 2, 3, 4}}
		b, err := d.Marshal()
		So(err, ShouldBeNil)
		So(b, ShouldResemble, d.DevAddr[:])

		So(d.Size(), ShouldEqual, 4)

		b2 := make([]byte, d.Size())
		i, err := d.MarshalTo(b2)
		So(err, ShouldBeNil)
		So(i, ShouldEqual, d.Size())

		var d2 DevAddr
		So(d2.Unmarshal(b), ShouldBeNil)
		So(d2, ShouldResemble, d)
	})
}
