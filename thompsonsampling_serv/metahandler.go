package thompsonsampling_serv

import (
	"context"

	"github.com/featmate/thompsonsampling-orderrpc/thompsonsampling_pb"
	"google.golang.org/protobuf/types/known/emptypb"
)

// GetInfo 获取业务key的信息
func (s *Server) GetMeta(ctx context.Context, in *emptypb.Empty) (*thompsonsampling_pb.MetaResponse, error) {
	return &thompsonsampling_pb.MetaResponse{
		RedisURL:                    s.RedisURL,
		Redis_RouteMod:              s.Redis_RouteMod,
		QueryRedisTimeout:           int64(s.QueryRedisTimeout),
		DefaultKeyTTL:               int64(s.DefaultKeyTTL),
		ScopeObserveMode:            s.ScopeObserveMode,
		ScopeObserveModeEtcdurl:     s.ScopeObserveMode_ETCDURL,
		ScopeObserveModeConntimeout: int64(s.ScopeObserveMode_ConnTimeout),
		NamespaceSetting: &thompsonsampling_pb.NamespaceSetting{
			ALGO:                s.Namespace_ALGO,
			ALGOMeta:            s.Namespace_ALGOMeta,
			DefaultBusinessName: s.Namespace_DefaultBusinessName,
			DefaultTargetName:   s.Namespace_DefaultTargetName,
		},
	}, nil
}
