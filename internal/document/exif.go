package document

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strings"

	"github.com/gen2brain/h265/heic"
)

// exifPayload finds the TIFF structure that carries a photograph's Exif tags.
func exifPayload(format string, data []byte) []byte {
	switch format {
	case "tiff":
		return data
	case "heic":
		payload, err := heic.RawExif(bytes.NewReader(data))
		if err != nil {
			return nil
		}
		return payload
	case "jpeg":
		for i := 2; i+4 <= len(data) && data[i] == 0xff; {
			marker, size := data[i+1], int(binary.BigEndian.Uint16(data[i+2:]))
			if marker == 0xda || marker == 0xd9 || size < 2 || i+2+size > len(data) {
				return nil
			}
			if segment := data[i+4 : i+2+size]; marker == 0xe1 && bytes.HasPrefix(segment, []byte("Exif\x00\x00")) {
				return segment[6:]
			}
			i += 2 + size
		}
	}
	return nil
}

type tiffTag struct {
	id, kind uint16
	value    []byte
	order    binary.ByteOrder
}

func (t tiffTag) text() string {
	if t.kind != 2 {
		return ""
	}
	s, _, _ := strings.Cut(string(t.value), "\x00")
	return strings.TrimSpace(s)
}

func (t tiffTag) number(i int) float64 {
	switch {
	case t.kind == 3 && len(t.value) >= 2*i+2:
		return float64(t.order.Uint16(t.value[2*i:]))
	case t.kind == 4 && len(t.value) >= 4*i+4:
		return float64(t.order.Uint32(t.value[4*i:]))
	case (t.kind == 5 || t.kind == 10) && len(t.value) >= 8*i+8:
		n, d := t.order.Uint32(t.value[8*i:]), t.order.Uint32(t.value[8*i+4:])
		if d == 0 {
			return 0
		}
		if t.kind == 10 {
			return float64(int32(n)) / float64(int32(d))
		}
		return float64(n) / float64(d)
	}
	return 0
}

// tiffDirectory reads one directory of tags. Offsets and counts come from the
// file, so every one is checked against the payload before it is followed.
func tiffDirectory(data []byte, order binary.ByteOrder, offset uint32) []tiffTag {
	sizes := map[uint16]uint64{1: 1, 2: 1, 3: 2, 4: 4, 5: 8, 7: 1, 9: 4, 10: 8}
	if uint64(offset)+2 > uint64(len(data)) {
		return nil
	}
	n := int(order.Uint16(data[offset:]))
	var tags []tiffTag
	for i := 0; i < min(n, 512); i++ {
		at := uint64(offset) + 2 + 12*uint64(i)
		if at+12 > uint64(len(data)) {
			break
		}
		entry := data[at : at+12]
		tag := tiffTag{id: order.Uint16(entry), kind: order.Uint16(entry[2:]), order: order}
		size := sizes[tag.kind] * uint64(order.Uint32(entry[4:]))
		switch {
		case size == 0 || size > 64<<10:
			continue
		case size <= 4:
			tag.value = entry[8 : 8+size]
		default:
			start := uint64(order.Uint32(entry[8:]))
			if start+size > uint64(len(data)) {
				continue
			}
			tag.value = data[start : start+size]
		}
		tags = append(tags, tag)
	}
	return tags
}

func exifFields(payload []byte) []Field {
	if len(payload) < 8 {
		return nil
	}
	var order binary.ByteOrder
	switch string(payload[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return nil
	}
	if order.Uint16(payload[2:]) != 42 {
		return nil
	}
	var maker, model, lens, taken, written, software, artist, copyright, location string
	var orientation, exposure, aperture, iso, focal float64
	var photo, gps uint32
	for _, t := range tiffDirectory(payload, order, order.Uint32(payload[4:])) {
		switch t.id {
		case 0x010f:
			maker = t.text()
		case 0x0110:
			model = t.text()
		case 0x0112:
			orientation = t.number(0)
		case 0x0131:
			software = t.text()
		case 0x0132:
			written = t.text()
		case 0x013b:
			artist = t.text()
		case 0x8298:
			copyright = t.text()
		case 0x8769:
			photo = uint32(t.number(0))
		case 0x8825:
			gps = uint32(t.number(0))
		}
	}
	// Directories are read one level deep, so a file cannot make them loop.
	if photo != 0 {
		for _, t := range tiffDirectory(payload, order, photo) {
			switch t.id {
			case 0x829a:
				exposure = t.number(0)
			case 0x829d:
				aperture = t.number(0)
			case 0x8827:
				iso = t.number(0)
			case 0x9003:
				taken = t.text()
			case 0x920a:
				focal = t.number(0)
			case 0xa434:
				lens = t.text()
			}
		}
	}
	if gps != 0 {
		var ref [2]string
		var degrees [2]float64
		var found [2]bool
		for _, t := range tiffDirectory(payload, order, gps) {
			switch t.id {
			case 1, 3:
				ref[t.id/2] = t.text()
			case 2, 4:
				degrees[t.id/2-1] = t.number(0) + t.number(1)/60 + t.number(2)/3600
				found[t.id/2-1] = len(t.value) >= 24
			}
		}
		valid := func(v, limit float64) bool { return !math.IsNaN(v) && v >= 0 && v <= limit }
		if found[0] && found[1] && ref[0] != "" && ref[1] != "" && valid(degrees[0], 90) && valid(degrees[1], 180) {
			location = fmt.Sprintf("%.5f° %s, %.5f° %s", degrees[0], ref[0], degrees[1], ref[1])
		}
	}
	if taken == "" {
		taken = written
	}
	// "2026:03:04 05:06:07" separates the date with colons.
	if len(taken) >= 10 && taken[4] == ':' && taken[7] == ':' {
		taken = taken[:4] + "-" + taken[5:7] + "-" + taken[8:]
	}
	if maker != "" && !strings.HasPrefix(strings.ToLower(model), strings.ToLower(maker)) {
		model = strings.TrimSpace(maker + " " + model)
	}
	var settings []string
	switch {
	case exposure >= 1:
		settings = append(settings, fmt.Sprintf("%.4g s", exposure))
	case exposure > 0:
		settings = append(settings, fmt.Sprintf("1/%.0f s", 1/exposure))
	}
	if aperture > 0 {
		settings = append(settings, fmt.Sprintf("f/%.3g", aperture))
	}
	if iso > 0 {
		settings = append(settings, fmt.Sprintf("ISO %.0f", iso))
	}
	if focal > 0 {
		settings = append(settings, fmt.Sprintf("%.3g mm", focal))
	}
	turned := map[float64]string{2: "mirrored", 3: "rotated 180°", 4: "flipped", 5: "mirrored, rotated 90° counterclockwise", 6: "rotated 90° clockwise", 7: "mirrored, rotated 90° clockwise", 8: "rotated 90° counterclockwise"}[orientation]
	return Section("Photo",
		Field{"Camera", model}, Field{"Lens", lens}, Field{"Taken", taken}, Field{"Exposure", strings.Join(settings, " · ")},
		Field{"Orientation", turned}, Field{"Location", location}, Field{"Software", software}, Field{"Artist", artist}, Field{"Copyright", copyright})
}
