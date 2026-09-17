package images

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"

	"github.com/disintegration/imaging"
)

func CompressProfileImage(buffer []byte, zyid string) (string, error) {

	const maxSize = 50 * 1024
	const targetSize = 50 * 1024 // 50KB
	const targetWidth, targetHeight = 63, 63

	// Decode the image
	img, _, err := image.Decode(bytes.NewReader(buffer))
	if err != nil {
		return "", err
	}
	fmt.Println("image decoded")

	if len(buffer) <= maxSize {
		// Image is less than 50KB, no compression needed
		img = imaging.Resize(img, targetWidth, targetHeight, imaging.Lanczos)
		fmt.Println("image is lessthan 50kb so resizing it")
		return base64.StdEncoding.EncodeToString(buffer), nil
	}

	// Resize only if the image is larger than the target dimensions
	img = imaging.Resize(img, targetWidth, targetHeight, imaging.Lanczos)

	// Create a buffer to store the compressed image
	var compressedBuffer bytes.Buffer

	// Compress the image with initial quality
	quality := 70
	err = jpeg.Encode(&compressedBuffer, img, &jpeg.Options{Quality: quality})
	if err != nil {
		return "", err
	}

	// Convert the final image to Base64
	imageBase64 := base64.StdEncoding.EncodeToString(compressedBuffer.Bytes())
	fmt.Println("Published image for user", zyid)

	return imageBase64, nil
}
