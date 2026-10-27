package streamdeck

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
)

// ErrNoTouchStrip is returned by the touch strip methods on a model without one.
var ErrNoTouchStrip = errors.New("this stream deck has no touch strip")

const (
	touchStripReportSize = 1024
	touchStripHeaderSize = 16
)

// FillTouchStrip fills the whole touch strip with an image. An image of another size is
// resized to the strip first.
func (sd *StreamDeck) FillTouchStrip(img image.Image) error {
	if !sd.Config.HasTouchStrip() {
		return ErrNoTouchStrip
	}

	rect := img.Bounds()
	if rect.Dx() != sd.Config.TouchStripWidth || rect.Dy() != sd.Config.TouchStripHeight {
		img = resize(img, sd.Config.TouchStripWidth, sd.Config.TouchStripHeight)
	}

	return sd.FillTouchStripRegion(0, 0, img)
}

// FillTouchStripRegion draws an image onto the touch strip with its top-left corner at x, y.
// The image is sent at its own size and must fit inside the strip; the rest of the strip is
// left as it was.
func (sd *StreamDeck) FillTouchStripRegion(x, y int, img image.Image) error {
	if !sd.Config.HasTouchStrip() {
		return ErrNoTouchStrip
	}

	rect := img.Bounds()
	w, h := rect.Dx(), rect.Dy()
	if x < 0 || y < 0 || w < 1 || h < 1 || x+w > sd.Config.TouchStripWidth || y+h > sd.Config.TouchStripHeight {
		return fmt.Errorf("touch strip region %dx%d at %d,%d does not fit the %dx%d strip",
			w, h, x, y, sd.Config.TouchStripWidth, sd.Config.TouchStripHeight)
	}

	buf := bytes.Buffer{}
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		return err
	}

	sd.lock.Lock()
	defer sd.lock.Unlock()

	for _, report := range touchStripReports(x, y, w, h, buf.Bytes()) {
		n, err := sd.device.Write(report)
		if err != nil {
			return err
		}
		if n != len(report) {
			return fmt.Errorf("only wrote %d of %d", n, len(report))
		}
	}

	return nil
}

// ClearTouchStrip fills the touch strip with black.
func (sd *StreamDeck) ClearTouchStrip() error {
	if !sd.Config.HasTouchStrip() {
		return ErrNoTouchStrip
	}

	img := image.NewRGBA(image.Rect(0, 0, sd.Config.TouchStripWidth, sd.Config.TouchStripHeight))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.Black), image.Point{}, draw.Src)

	return sd.FillTouchStripRegion(0, 0, img)
}

// touchStripReports splits a JPEG into the 1024-byte output reports the touch strip takes.
// Each report starts with a 16-byte header: 0x02 0x0C, then x, y, width and height as
// little-endian uint16, a last-report flag, the page number and the payload length as
// little-endian uint16, and one padding byte.
func touchStripReports(x, y, w, h int, payload []byte) [][]byte {
	const chunk = touchStripReportSize - touchStripHeaderSize

	reports := [][]byte{}
	pos := 0
	for page := 0; ; page++ {
		n := min(chunk, len(payload)-pos)
		isLast := pos+n == len(payload)

		report := make([]byte, touchStripReportSize)
		report[0] = 0x02
		report[1] = 0x0C
		binary.LittleEndian.PutUint16(report[2:], uint16(x))
		binary.LittleEndian.PutUint16(report[4:], uint16(y))
		binary.LittleEndian.PutUint16(report[6:], uint16(w))
		binary.LittleEndian.PutUint16(report[8:], uint16(h))
		if isLast {
			report[10] = 1
		}
		binary.LittleEndian.PutUint16(report[11:], uint16(page))
		binary.LittleEndian.PutUint16(report[13:], uint16(n))
		copy(report[touchStripHeaderSize:], payload[pos:pos+n])

		reports = append(reports, report)
		pos += n
		if isLast {
			return reports
		}
	}
}
