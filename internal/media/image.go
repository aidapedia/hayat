package media

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // registers WebP with image.Decode

	"github.com/aidapedia/hayat/internal/errs"
)

// MaxUploadBytes is the largest file Hayat accepts. What it keeps is smaller:
// every image is re-encoded at a fixed size.
const MaxUploadBytes = 5 << 20

// maxPixels refuses images whose header claims more than this, before
// decoding them: a tiny file can describe an enormous image.
const maxPixels = 40_000_000

var ErrBadImage = errs.New("upload a PNG, JPEG or WebP image of at most 5 MB")

// Kind says how an image is used, and so how it is sized.
type Kind string

const (
	// Avatar is a person's picture: cropped square, 256×256.
	Avatar Kind = "avatar"
	// Logo is a tenant's, a service's or a login page's mark: fits in
	// 512×512, keeps its shape and transparency.
	Logo Kind = "logo"
)

// prepared is an image ready to keep.
type prepared struct {
	data          []byte
	contentType   string
	ext           string
	width, height int
}

// prepare decodes an upload and writes a fresh image from its pixels only.
// Nothing else survives, EXIF (camera, GPS location) included, and nothing
// that isn't an image (SVG, HTML) is ever kept.
func prepare(kind Kind, data []byte) (prepared, error) {
	if len(data) == 0 || len(data) > MaxUploadBytes {
		return prepared{}, ErrBadImage
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg" && format != "webp") || cfg.Width*cfg.Height > maxPixels || cfg.Width == 0 || cfg.Height == 0 {
		return prepared{}, ErrBadImage
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return prepared{}, ErrBadImage
	}

	var dst *image.RGBA
	switch kind {
	case Avatar:
		// Crop the largest centered square, then scale to 256.
		b := src.Bounds()
		side := min(b.Dx(), b.Dy())
		crop := image.Rect(0, 0, side, side).Add(image.Pt(b.Min.X+(b.Dx()-side)/2, b.Min.Y+(b.Dy()-side)/2))
		size := min(256, side)
		dst = image.NewRGBA(image.Rect(0, 0, size, size))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, crop, draw.Src, nil)
	default:
		b := src.Bounds()
		w, h := b.Dx(), b.Dy()
		if w > 512 || h > 512 { // never enlarge
			if w >= h {
				w, h = 512, max(1, h*512/b.Dx())
			} else {
				w, h = max(1, w*512/b.Dy()), 512
			}
		}
		dst = image.NewRGBA(image.Rect(0, 0, w, h))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
	}

	var out bytes.Buffer
	p := prepared{width: dst.Bounds().Dx(), height: dst.Bounds().Dy()}
	if kind == Avatar && dst.Opaque() {
		// A photo: JPEG is a fraction of the size.
		flat := image.NewRGBA(dst.Bounds())
		draw.Draw(flat, flat.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
		draw.Draw(flat, flat.Bounds(), dst, image.Point{}, draw.Over)
		if err := jpeg.Encode(&out, flat, &jpeg.Options{Quality: 88}); err != nil {
			return prepared{}, err
		}
		p.contentType, p.ext = "image/jpeg", "jpg"
	} else {
		enc := png.Encoder{CompressionLevel: png.BestCompression}
		if err := enc.Encode(&out, dst); err != nil {
			return prepared{}, err
		}
		p.contentType, p.ext = "image/png", "png"
	}
	p.data = out.Bytes()
	return p, nil
}
