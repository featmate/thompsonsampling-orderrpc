package thompsonsampling

import (
	"context"
	"time"

	log "github.com/Golang-Tools/loggerhelper"
	rp "github.com/Golang-Tools/redishelper/proxy"
	"github.com/go-redis/redis/v8"
)

func (s *Server) ResetParamToRedis(business_namespcae, target_namespacestring string, ttl time.Duration, infos ...*CandidateUpdateInfo) ([]*CandidateUpdateInfo, error) {
	ctx, cancel := s.QueryRedisCtx()
	defer cancel()
	pipe := rp.Proxy.TxPipeline()
	for _, info := range infos {
		key := BuildKey(business_namespcae, target_namespacestring, info.Candidate)
		pipe.HSet(ctx, key, "alpha", info.Alpha, "beta", info.Beta)
		pipe.Expire(ctx, key, ttl)
	}
	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, err
	}
	return infos, nil
}

func (s *Server) IncrParamToRedis(business_namespcae, target_namespacestring string, ttl time.Duration, infos ...*CandidateUpdateInfo) ([]*CandidateUpdateInfo, error) {
	ctx, cancel := s.QueryRedisCtx()
	defer cancel()
	alphafus := map[string]*redis.FloatCmd{}
	betafus := map[string]*redis.FloatCmd{}
	pipe := rp.Proxy.TxPipeline()
	for _, info := range infos {
		key := BuildKey(business_namespcae, target_namespacestring, info.Candidate)
		alphafus[info.Candidate] = pipe.HIncrByFloat(ctx, key, "alpha", info.Alpha)
		betafus[info.Candidate] = pipe.HIncrByFloat(ctx, key, "beta", info.Beta)
		pipe.Expire(ctx, key, ttl)
	}
	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, err
	}
	result := []*CandidateUpdateInfo{}
	for _, info := range infos {
		a, err := alphafus[info.Candidate].Result()
		if err != nil {
			return nil, err
		}
		b, err := betafus[info.Candidate].Result()
		if err != nil {
			return nil, err
		}
		result = append(result, &CandidateUpdateInfo{
			Alpha: a,
			Beta:  b,
		})
	}

	return result, nil
}

//Update
func (s *Server) Update(ctx context.Context, in *UpdateQuery) (*UpdateResponse, error) {
	log.Info("Rank get message ", log.Dict{"in": in})
	var result []*CandidateUpdateInfo
	var err error
	switch in.UpdateType {
	case UpdateQuery_RESET:
		{
			if in.Ttl >= 1 {
				result, err = s.ResetParamToRedis(in.BusinessNamespace, in.TargetNamespace, time.Duration(in.Ttl)*time.Second, in.CandidateUpdateInfo...)
			} else {
				result, err = s.ResetParamToRedis(in.BusinessNamespace, in.TargetNamespace, time.Duration(s.DefaultKeyTTL)*time.Second, in.CandidateUpdateInfo...)
			}
		}
	default:
		{
			if in.Ttl >= 1 {
				result, err = s.IncrParamToRedis(in.BusinessNamespace, in.TargetNamespace, time.Duration(in.Ttl)*time.Second, in.CandidateUpdateInfo...)
			} else {
				result, err = s.IncrParamToRedis(in.BusinessNamespace, in.TargetNamespace, time.Duration(s.DefaultKeyTTL)*time.Second, in.CandidateUpdateInfo...)
			}
		}
	}
	if err != nil {
		return nil, err
	}
	return &UpdateResponse{
		CandidateUpdateInfo: result,
	}, nil
}
