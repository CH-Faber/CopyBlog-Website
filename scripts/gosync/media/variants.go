package media

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"path"
	"strings"
)

// EncodeVariant keeps the source format where practical and bounds the image
// dimensions before it is served by the public site. GIFs are kept as-is so
// animated captures do not silently become a single frame.
func EncodeVariant(data []byte, filename string, maxDimension, quality int) ([]byte, error) {
	if maxDimension <= 0 {
		return nil, fmt.Errorf("invalid image dimension: %d", maxDimension)
	}
	ext := strings.ToLower(path.Ext(filename))
	if ext == ".gif" {
		return data, nil
	}
	decoded, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	resized := fit(decoded, maxDimension)
	var output bytes.Buffer
	switch format {
	case "png":
		err = png.Encode(&output, resized)
	case "jpeg":
		if quality <= 0 || quality > 100 {
			quality = 82
		}
		err = jpeg.Encode(&output, resized, &jpeg.Options{Quality: quality})
	default:
		// Decode may identify uncommon extensions by content. JPEG is the
		// broadly compatible fallback for the static site.
		err = jpeg.Encode(&output, resized, &jpeg.Options{Quality: quality})
	}
	if err != nil {
		return nil, fmt.Errorf("encode %s image: %w", format, err)
	}
	return output.Bytes(), nil
}

func fit(source image.Image, maxDimension int) image.Image {
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= maxDimension && height <= maxDimension {
		return source
	}
	scale := float64(maxDimension) / float64(width)
	if height > width {
		scale = float64(maxDimension) / float64(height)
	}
	newWidth := max(1, int(float64(width)*scale))
	newHeight := max(1, int(float64(height)*scale))
	destination := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	for y := 0; y < newHeight; y++ {
		sourceY := bounds.Min.Y + y*height/newHeight
		for x := 0; x < newWidth; x++ {
			sourceX := bounds.Min.X + x*width/newWidth
			destination.Set(x, y, source.At(sourceX, sourceY))
		}
	}
	return destination
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
