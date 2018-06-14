package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/brocaar/lorawan"
)

func TestAES128Key(t *testing.T) {
	Convey("Testing an AES128Key", t, func() {
		k := AES128Key{AES128Key: lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8}}
		b, err := k.Marshal()
		So(err, ShouldBeNil)
		So(b, ShouldResemble, k.AES128Key[:])

		So(k.Size(), ShouldEqual, 16)

		b2 := make([]byte, k.Size())
		i, err := k.MarshalTo(b2)
		So(err, ShouldBeNil)
		So(i, ShouldEqual, k.Size())
		So(b2, ShouldResemble, b)

		var k2 AES128Key
		So(k2.Unmarshal(b), ShouldBeNil)
		So(k2, ShouldResemble, k)
	})
}
