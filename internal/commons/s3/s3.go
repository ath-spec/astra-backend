package s3

import (
	"bytes"
	"net/http"
	"github.com/yourusername/astra-backend/internal/commons/util"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

type s3Config struct {
	Bucket          string
	Region          string
	AccessKeyId     string
	SecretAccessKey string
}

type S3Interface interface {
	UploadFile(buffer []byte, fileName string) (string, error)
	CreateBuffer() *aws.WriteAtBuffer
	DownloadFile(buff *aws.WriteAtBuffer, fileName string) error
	DeleteFile(fileName string) error
}

// Connection for AWS S3.
type S3Conn struct {
	Config  s3Config
	Session *session.Session
}

func CreateAWSSession(bucket, region, accessKey, secretKey string) (*S3Conn, error) {
	config := s3Config{
		Bucket:          bucket,
		Region:          region,
		AccessKeyId:     accessKey,
		SecretAccessKey: secretKey,
	}

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(config.Region),
		Credentials: credentials.NewStaticCredentials(
			config.AccessKeyId,
			config.SecretAccessKey,
			"",
		),
	})
	if err != nil {
		return nil, err
	}

	return &S3Conn{
		Session: sess,
		Config:  config,
	}, nil
}

// UploadFile uploads a file to S3.
func (s *S3Conn) UploadFile(buffer []byte, fileName string) (string, error) {
	uploader := s3manager.NewUploader(s.Session)
	_, err := uploader.Upload(&s3manager.UploadInput{
		Bucket:               aws.String(s.Config.Bucket),
		Key:                  aws.String(fileName),
		Body:                 bytes.NewReader(buffer),
		ContentType:          aws.String(http.DetectContentType(buffer)),
		ContentDisposition:   aws.String("attachment"),
		ServerSideEncryption: aws.String("AES256"),
		StorageClass:         aws.String("INTELLIGENT_TIERING"),
		ACL:                  aws.String("private"),
	})
	if err != nil {
		return "", err
	}

	return fileName, nil
}

// CreateBuffer creates a buffer for file downloading.
func (s *S3Conn) CreateBuffer() *aws.WriteAtBuffer {
	return &aws.WriteAtBuffer{}
}

// DownloadFile downloads a file from S3 to the given buffer.
func (s *S3Conn) DownloadFile(buff *aws.WriteAtBuffer, fileName string) error {
	downloader := s3manager.NewDownloader(s.Session)
	_, err := downloader.Download(buff, &s3.GetObjectInput{
		Bucket: aws.String(s.Config.Bucket),
		Key:    aws.String(fileName),
	})
	if aerr, ok := err.(awserr.Error); ok {
		if aerr.Code() == "NoSuchKey" {
			return util.ErrEmptyResult
		}
	}
	return err
}

func (s *S3Conn) DeleteFile(fileName string) error {
	svc := s3.New(s.Session)
	_, err := svc.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(s.Config.Bucket),
		Key:    aws.String(fileName),
	})

	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == "NoSuchKey" {
				return util.ErrEmptyResult
			}
		}
		return err
	}

	// Wait until the object is deleted
	err = svc.WaitUntilObjectNotExists(&s3.HeadObjectInput{
		Bucket: aws.String(s.Config.Bucket),
		Key:    aws.String(fileName),
	})

	if err != nil {
		return err
	}

	return nil
}
