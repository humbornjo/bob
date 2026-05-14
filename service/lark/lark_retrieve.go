package larksvc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/chyroc/lark"
	"github.com/google/uuid"
)

var mcache sync.Map

func (s *Service) RetrieveLarkUser(ctx context.Context, id string, idType lark.IDType,
) (userID string, userName string, err error) {
	if v, ok := mcache.Load(id); ok {
		if user, ok := v.(*lark.GetUserRespUser); ok {
			return user.UserID, user.Name, nil
		}
	}
	resp, _, err := s.larkcli.Contact.GetUser(ctx, &lark.GetUserReq{
		UserID:     id,
		UserIDType: new(idType),
	})
	if err != nil {
		return "", "", err
	}
	mcache.Store(id, resp.User)
	return resp.User.UserID, resp.User.Name, nil

}

func (s *Service) RetrieveLarkApp(ctx context.Context, id string, idType lark.IDType,
) (appID string, appName string, err error) {
	if v, ok := mcache.Load(id); ok {
		if app, ok := v.(*lark.GetApplicationRespApp); ok {
			return app.AppID, app.AppName, nil
		}
	}
	resp, _, err := s.larkcli.Application.GetApplication(ctx, &lark.GetApplicationReq{
		AppID: id,
		Lang:  "en_us",
	})
	if err != nil {
		return "", "", err
	}
	mcache.Store(id, resp.App)
	return resp.App.AppID, resp.App.AppName, nil
}

func (s *Service) RetrieveLarkImageURL(ctx context.Context, imageKey string) (url string, err error) {
	resp, _, err := s.larkcli.File.DownloadImage(ctx, &lark.DownloadImageReq{ImageKey: imageKey})
	if err != nil {
		return "", err
	}

	key, err := s.fs.Upload(ctx, resp.File, fmt.Sprintf("%d@%s", time.Now().Nanosecond(), uuid.New()))
	if err != nil {
		return "", err
	}
	return s.fs.PresignURL(ctx, key)
}
