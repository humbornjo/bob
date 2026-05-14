package storage

import (
	"context"
	"fmt"
	"io"
	"path"

	"github.com/volcengine/ve-tos-golang-sdk/v2/tos"
	"github.com/volcengine/ve-tos-golang-sdk/v2/tos/enum"
)

type Storage interface {
	FileSystem
}

type FileSystem interface {
	PresignURL(ctx context.Context, key string) (url string, err error)
	Upload(ctx context.Context, file io.Reader, fileName string) (key string, err error)
}

func NewFileSystem(cli *tos.ClientV2, bucket string, folder string) Storage {
	return &fsTOS{ClientV2: cli}
}

type fsTOS struct {
	*tos.ClientV2

	folder     string
	bucketName string
}

func (s *fsTOS) convError(err error) error {
	if serverErr, ok := err.(*tos.TosServerError); ok {
		err = fmt.Errorf(
			"tos server error: %s, requestID: %s, statusCode: %d, code: %s, message: %s",
			serverErr.Error(), serverErr.RequestID, serverErr.StatusCode, serverErr.Code, serverErr.Message,
		)
	} else if clientErr, ok := err.(*tos.TosClientError); ok {
		err = fmt.Errorf("tos client error: %s, cause: %s", clientErr.Error(), clientErr.Cause.Error())
	}
	return err
}

func (s *fsTOS) PresignURL(ctx context.Context, key string) (url string, err error) {
	resp, err := s.PreSignedURL(&tos.PreSignedURLInput{
		HTTPMethod: enum.HttpMethodGet,
		Key:        key,
		Bucket:     s.bucketName,
	})
	if err != nil {
		return "", s.convError(err)
	}
	return resp.SignedUrl, nil
}

func (s *fsTOS) Upload(ctx context.Context, file io.Reader, fileName string) (key string, err error) {
	key = path.Join(s.folder, fileName)
	if _, err = s.PutObjectV2(ctx, &tos.PutObjectV2Input{
		Content:             file,
		PutObjectBasicInput: tos.PutObjectBasicInput{Key: key, Bucket: s.bucketName},
	}); err != nil {
		return "", s.convError(err)
	}

	return key, nil
}
