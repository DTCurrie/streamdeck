package streamdeck

import (
	"encoding/binary"
	"image"
	"testing"

	"go.viam.com/test"
)

func TestTouchStripReportsSplitsPayload(t *testing.T) {
	payload := make([]byte, 2500)
	for i := range payload {
		payload[i] = byte(i)
	}

	reports := touchStripReports(200, 0, 200, 100, payload)

	test.That(t, len(reports), test.ShouldEqual, 3)
	for i, r := range reports {
		test.That(t, len(r), test.ShouldEqual, 1024)
		test.That(t, r[0], test.ShouldEqual, 0x02)
		test.That(t, r[1], test.ShouldEqual, 0x0C)
		test.That(t, binary.LittleEndian.Uint16(r[2:]), test.ShouldEqual, 200)
		test.That(t, binary.LittleEndian.Uint16(r[4:]), test.ShouldEqual, 0)
		test.That(t, binary.LittleEndian.Uint16(r[6:]), test.ShouldEqual, 200)
		test.That(t, binary.LittleEndian.Uint16(r[8:]), test.ShouldEqual, 100)
		test.That(t, binary.LittleEndian.Uint16(r[11:]), test.ShouldEqual, i)
	}

	test.That(t, reports[0][10], test.ShouldEqual, 0)
	test.That(t, binary.LittleEndian.Uint16(reports[0][13:]), test.ShouldEqual, 1008)
	test.That(t, reports[0][16], test.ShouldEqual, 0)
	test.That(t, reports[1][10], test.ShouldEqual, 0)
	test.That(t, binary.LittleEndian.Uint16(reports[1][13:]), test.ShouldEqual, 1008)
	test.That(t, reports[1][16], test.ShouldEqual, byte(1008%256))
	test.That(t, reports[2][10], test.ShouldEqual, 1)
	test.That(t, binary.LittleEndian.Uint16(reports[2][13:]), test.ShouldEqual, 484)
	test.That(t, reports[2][16], test.ShouldEqual, byte(2016%256))
	test.That(t, reports[2][16+484], test.ShouldEqual, 0)
}

func TestTouchStripReportsSingleReport(t *testing.T) {
	reports := touchStripReports(0, 0, 800, 100, []byte{1, 2, 3})

	test.That(t, len(reports), test.ShouldEqual, 1)
	test.That(t, reports[0][10], test.ShouldEqual, 1)
	test.That(t, binary.LittleEndian.Uint16(reports[0][13:]), test.ShouldEqual, 3)
}

func TestTouchStripRejectsModelsWithoutOne(t *testing.T) {
	sd := &StreamDeck{Config: &Original2}
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))

	test.That(t, sd.FillTouchStrip(img), test.ShouldEqual, ErrNoTouchStrip)
	test.That(t, sd.FillTouchStripRegion(0, 0, img), test.ShouldEqual, ErrNoTouchStrip)
	test.That(t, sd.ClearTouchStrip(), test.ShouldEqual, ErrNoTouchStrip)
}

func TestTouchStripRegionMustFit(t *testing.T) {
	sd := &StreamDeck{Config: &Plus}

	err := sd.FillTouchStripRegion(700, 0, image.NewRGBA(image.Rect(0, 0, 200, 100)))
	test.That(t, err, test.ShouldNotBeNil)
	test.That(t, err.Error(), test.ShouldContainSubstring, "does not fit")

	err = sd.FillTouchStripRegion(0, 50, image.NewRGBA(image.Rect(0, 0, 200, 51)))
	test.That(t, err, test.ShouldNotBeNil)

	err = sd.FillTouchStripRegion(-1, 0, image.NewRGBA(image.Rect(0, 0, 200, 100)))
	test.That(t, err, test.ShouldNotBeNil)
}

func TestPlusHasTouchStrip(t *testing.T) {
	test.That(t, Plus.HasTouchStrip(), test.ShouldBeTrue)
	test.That(t, Plus.NumDials, test.ShouldEqual, 4)
	test.That(t, Original.HasTouchStrip(), test.ShouldBeFalse)
	test.That(t, Original2.HasTouchStrip(), test.ShouldBeFalse)
}
