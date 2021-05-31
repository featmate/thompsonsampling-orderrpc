package thompsonsampling

import (
	"context"
	"time"

	log "github.com/Golang-Tools/loggerhelper"
	rp "github.com/Golang-Tools/redishelper/proxy"
)

func (s *Server) getMetaFromRedis(business_namespcae, target_namespacestring string) ([]string, time.Duration, error) {
	ctx, cancel := s.QueryRedisCtx()
	defer cancel()
	metakey := BuildMetaKey(business_namespcae, target_namespacestring)
	candidates, err := rp.Proxy.SMembers(ctx, metakey).Result()
	if err != nil {
		return nil, 0, err
	}
	ttl, err := rp.Proxy.TTL(ctx, metakey).Result()
	if err != nil {
		return nil, 0, err
	}
	return candidates, ttl, nil
}

//Rank
func (s *Server) Meta(ctx context.Context, in *MetaQuery) (*MetaResponse, error) {
	log.Info("Meta get message ", log.Dict{"in": in})
	candidates, ttl, err := s.getMetaFromRedis(in.BusinessNamespace, in.TargetNamespace)
	if err != nil {
		return nil, err
	}
	ttl.Seconds()
	return &MetaResponse{Candidates: candidates, Ttl: int64(ttl.Seconds())}, nil
}
