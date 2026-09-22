package s3

import (
	"github.com/aws/aws-sdk-go/aws"
)

type S3ConnTest struct {
}

func CreateAWSSessionTest() S3Interface {
	return &S3ConnTest{}
}

func (s *S3ConnTest) UploadFile(buffer []byte, fileName string) (string, error) {
	return "", nil
}

func (s *S3ConnTest) CreateBuffer() *aws.WriteAtBuffer {
	return &aws.WriteAtBuffer{}
}

func (s *S3ConnTest) DownloadFile(buff *aws.WriteAtBuffer, fileName string) error {
	return nil
}

func (s *S3ConnTest) DeleteFile(fileName string) error {
	return nil
}
