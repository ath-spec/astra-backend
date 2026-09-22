package images

import (
	"image"
	"image/color"

	"github.com/nfnt/resize"
	"golang.org/x/image/draw"
)

func Compress(img image.Image, newWidth uint) image.Image {
	bounds := img.Bounds()
	width := bounds.Max.X
	height := bounds.Max.Y

	newHeight := uint(float64(height) / (float64(width) / float64(newWidth)))

	resizedImage := resize.Resize(newWidth, newHeight, img, resize.Lanczos3)

	rgbaImage := image.NewRGBA(resizedImage.Bounds())
	drawImage := image.NewUniform(color.White)
	draw.Draw(rgbaImage, rgbaImage.Bounds(), drawImage, image.Point{}, draw.Over)
	draw.ApproxBiLinear.Scale(rgbaImage, rgbaImage.Bounds(), resizedImage, resizedImage.Bounds(), draw.Over, nil)

	return rgbaImage
}
