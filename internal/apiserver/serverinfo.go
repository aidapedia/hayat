package apiserver

import (
	"context"

	"connectrpc.com/connect"

	authzv1 "github.com/aidapedia/hayat/gen/go/hayat/authz/v1"
	"github.com/aidapedia/hayat/internal/version"
)

// serverInfoService is served without the service-token interceptor: a
// service reads the version before it enrolls (it has no credentials yet) and
// on every start, and refuses a Hayat older than the minimum it declares.
type serverInfoService struct{}

func (serverInfoService) GetServerInfo(context.Context, *connect.Request[authzv1.GetServerInfoRequest]) (*connect.Response[authzv1.GetServerInfoResponse], error) {
	return connect.NewResponse(&authzv1.GetServerInfoResponse{Version: version.Version}), nil
}
