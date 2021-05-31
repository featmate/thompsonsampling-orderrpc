package thompsonsampling

import (
	"context"
	"errors"
	"time"

	log "github.com/Golang-Tools/loggerhelper"
	rp "github.com/Golang-Tools/redishelper/proxy"
)

func (s *Server) ResetParamToRedis(key string, alpha, beta float64, ttl time.Duration) (float64, float64, error) {
	ctx, cancel := s.QueryRedisCtx()
	defer cancel()
	pipe := rp.Proxy.TxPipeline()
	pipe.HSet(ctx, key, "alpha", alpha, "beta", beta)
	pipe.Expire(ctx, key, ttl)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0, 0, err
	}
	return alpha, beta, nil
}

func (s *Server) IncrParamToRedis(key string, alpha, beta float64) (float64, float64, error) {
	ctx, cancel := s.QueryRedisCtx()
	defer cancel()
	pipe := rp.Proxy.TxPipeline()
	alphaFut := pipe.HIncrByFloat(ctx, key, "alpha", alpha)
	betaFut := pipe.HIncrByFloat(ctx, key, "beta", beta)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0, 0, err
	}
	a, err := alphaFut.Result()
	if err != nil {
		return 0, 0, err
	}
	b, err := betaFut.Result()
	if err != nil {
		return 0, 0, err
	}
	return a, b, nil
}

//Update
func (s *Server) Update(ctx context.Context, in *UpdateQuery) (*UpdateResponse, error) {
	log.Info("Rank get message ", log.Dict{"in": in})
	if in.Candidate == "" {
		return nil, errors.New("Candidate empty")
	}
	key := BuildKey(in.BusinessNamespcae, in.TargetNamespace, in.Candidate)
	var alpha float64
	var beta float64
	var err error
	switch in.UpdateType {
	case UpdateQuery_RESET:
		{
			if in.Ttl >= 1 {
				alpha, beta, err = s.ResetParamToRedis(key, in.Alpha, in.Beta, time.Duration(in.Ttl)*time.Second)
			} else {
				alpha, beta, err = s.ResetParamToRedis(key, in.Alpha, in.Beta, time.Duration(s.DefaultKeyTTL)*time.Second)
			}
		}
	default:
		{
			alpha, beta, err = s.IncrParamToRedis(key, in.Alpha, in.Beta)
		}
	}
	if err != nil {
		return nil, err
	}
	return &UpdateResponse{
		Alpha: alpha,
		Beta:  beta,
	}, nil
}
