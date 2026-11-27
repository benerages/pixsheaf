package imaging

import (
	"bytes"
	"errors"
	"fmt"
	goimage "image"

	"golang.org/x/image/bmp"
)

// BMP is decoded and encoded in Go with golang.org/x/image/bmp, because the
// libvips build has no BMP loader or saver (ADR 0021). The decoder supports
// uncompressed 1, 2, 4, 8, 24 and 32-bit BMPs; RLE and other compressed
// variants are rejected as unsupported.

// maxBMPPixels bounds a decoded BMP. The Go decoder allocates the whole
// image (4 bytes per pixel) and the copy into libvips needs the same again,
// which has to fit in the job memory limit.
const maxBMPPixels = 32 << 20

func isBMP(buf []byte) bool {
	return len(buf) >= 2 && buf[0] == 'B' && buf[1] == 'M'
}

// inspectBMP reads only the BMP headers.
func inspectBMP(buf []byte) (Info, error) {
	cfg, err := bmp.DecodeConfig(bytes.NewReader(buf))
	if err != nil {
		return Info{}, bmpError(err)
	}
	if cfg.Width < 1 || cfg.Height < 1 {
		return Info{}, fmt.Errorf("%w: BMP without pixels", ErrDecode)
	}
	return Info{Format: "bmp", Width: cfg.Width, Height: cfg.Height, Pages: 1}, nil
}

// decodeBMP decodes a BMP whose header has been checked by inspectBMP.
func decodeBMP(buf []byte, info Info) (goimage.Image, error) {
	if int64(info.Width)*int64(info.Height) > maxBMPPixels {
		return nil, fmt.Errorf("%w: BMP input has %dx%d pixels, more than %d", ErrTooManyPixels, info.Width, info.Height, maxBMPPixels)
	}
	img, err := bmp.Decode(bytes.NewReader(buf))
	if err != nil {
		return nil, bmpError(err)
	}
	return img, nil
}

func bmpError(err error) error {
	if errors.Is(err, bmp.ErrUnsupported) {
		return fmt.Errorf("%w: %v", ErrUnsupported, err)
	}
	return fmt.Errorf("%w: %v", ErrDecode, err)
}

// encodeBMP writes a BMP. Process flattens transparency first, so the
// output is always 24-bit: the encoder would write alpha with the 40-byte
// header, which most readers (the Go decoder included) treat as opaque.
func encodeBMP(img goimage.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := bmp.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEncode, err)
	}
	return buf.Bytes(), nil
}
