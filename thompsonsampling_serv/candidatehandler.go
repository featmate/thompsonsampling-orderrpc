package thompsonsampling_serv

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/Golang-Tools/etcdhelper/etcdproxy"
	log "github.com/Golang-Tools/loggerhelper/v2"
	"github.com/Golang-Tools/redishelper/v2/redisproxy"
	"github.com/featmate/thompsonsampling-orderrpc/thompsonsampling_pb"
	"github.com/go-redis/redis/v8"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func (s *Server) getInfoFromRedis(business, target string, candidates ...string) (map[string]*thompsonsampling_pb.CandidateInfo, error) {

	betas, err := s.getParamFromRedis(business, target, candidates...)
	if err != nil {
		return nil, err
	}
	ttls, err := s.getTTLFromRedis(business, target, candidates...)
	if err != nil {
		return nil, err
	}

	result := map[string]*thompsonsampling_pb.CandidateInfo{}
	for _, candidate := range candidates {
		beta, ok := betas[candidate]
		if !ok {
			beta = &thompsonsampling_pb.BetaParam{
				Alpha: 0,
				Beta:  0,
			}
		}
		ttl, ok := ttls[candidate]
		if !ok {
			ttl = -1
		}
		result[candidate] = &thompsonsampling_pb.CandidateInfo{
			Param: beta,
			Ttl:   ttl,
		}
	}

	return result, nil
}

// GetCandidateInfo
func (s *Server) GetCandidateInfo(ctx context.Context, in *thompsonsampling_pb.CandidateInfoQuery) (*thompsonsampling_pb.CandidateInfoResponse, error) {
	Business := ""
	Target := ""
	if in.Scope != nil {
		Business = in.Scope.Business
		Target = in.Scope.Target
	}
	infos, err := s.getInfoFromRedis(Business, Target, in.Candidates...)
	if err != nil {
		return nil, err
	}
	return &thompsonsampling_pb.CandidateInfoResponse{Infos: infos}, nil
}

func (s *Server) syncToEtcd(business_namespcae, target_namespacestring, candidate string, seconds int64) (time.Duration, error) {
	ctx, cancel := etcdproxy.Default.NewCtx()
	defer cancel()
	ttl := time.Duration(0)
	key := s.InfoKey(business_namespcae, target_namespacestring, candidate)
	resp, err := etcdproxy.Default.KV.Get(ctx, key)
	if err != nil {
		return 0, err
	}
	if len(resp.Kvs) == 0 {
		// 不存在 创建
		resp, err := etcdproxy.Default.Grant(ctx, seconds)
		if err != nil {
			return 0, err
		}
		putopts := []clientv3.OpOption{}
		putopts = append(putopts, clientv3.WithLease(resp.ID))

		value := fmt.Sprintf("%d", seconds)
		_, err = etcdproxy.Default.KV.Put(ctx, key, value, putopts...)
		if err != nil {
			return 0, err
		}
	} else {
		//存在,刷新
		kv := resp.Kvs[0]
		lastttlseconds, err := strconv.ParseInt(string(kv.Value), 10, 64)
		if err != nil {
			return 0, err
		}
		ttl = time.Duration(lastttlseconds) * time.Second
		_, err = etcdproxy.Default.KeepAliveOnce(ctx, clientv3.LeaseID(kv.Lease))
		if err != nil {
			return 0, err
		}
	}
	return ttl, nil
}

func (s *Server) ResetParamToRedis(business_namespcae, target_namespacestring string, ttl time.Duration, infos ...*thompsonsampling_pb.CandidateUpdateInfo) ([]*thompsonsampling_pb.CandidateUpdateInfo, error) {

	candidates_ttls := map[string]time.Duration{}
	for _, info := range infos {
		candidates_ttls[info.Candidate] = ttl
	}
	if s.ScopeObserveMode {
		seconds := int64(ttl.Seconds())
		for candidate := range candidates_ttls {
			oldttl, err := s.syncToEtcd(business_namespcae, target_namespacestring, candidate, seconds)
			if err != nil {
				return nil, err
			}
			if oldttl > 0 {
				candidates_ttls[candidate] = oldttl
			}
		}
	}

	ctx, cancel := redisproxy.Default.NewCtx()
	defer cancel()
	pipe := redisproxy.Default.TxPipeline()
	// infokey := s.InfoKey(business_namespcae, target_namespacestring)
	// pipe.Del(ctx, infokey)
	for _, info := range infos {
		ttl := candidates_ttls[info.Candidate]
		key := s.Key(business_namespcae, target_namespacestring, info.Candidate)
		pipe.HSet(ctx, key, "alpha", info.Param.Alpha, "beta", info.Param.Beta)
		pipe.Expire(ctx, key, ttl)
	}
	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, err
	}
	return infos, nil
}

func (s *Server) IncrParamToRedis(business_namespcae, target_namespacestring string, ttl time.Duration, infos ...*thompsonsampling_pb.CandidateUpdateInfo) ([]*thompsonsampling_pb.CandidateUpdateInfo, error) {
	candidates_ttls := map[string]time.Duration{}
	for _, info := range infos {
		candidates_ttls[info.Candidate] = ttl
	}
	if s.ScopeObserveMode {
		seconds := int64(ttl.Seconds())
		for candidate := range candidates_ttls {
			oldttl, err := s.syncToEtcd(business_namespcae, target_namespacestring, candidate, seconds)
			if err != nil {
				return nil, err
			}
			if oldttl > 0 {
				candidates_ttls[candidate] = oldttl
			}
		}
	}

	ctx, cancel := redisproxy.Default.NewCtx()
	defer cancel()
	alphafus := map[string]*redis.FloatCmd{}
	betafus := map[string]*redis.FloatCmd{}
	pipe := redisproxy.Default.TxPipeline()
	for _, info := range infos {
		key := s.Key(business_namespcae, target_namespacestring, info.Candidate)
		alphafus[info.Candidate] = pipe.HIncrByFloat(ctx, key, "alpha", info.Param.Alpha)
		betafus[info.Candidate] = pipe.HIncrByFloat(ctx, key, "beta", info.Param.Beta)
		pipe.Expire(ctx, key, ttl)
	}
	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, err
	}
	result := []*thompsonsampling_pb.CandidateUpdateInfo{}
	for _, info := range infos {
		a, err := alphafus[info.Candidate].Result()
		if err != nil {
			return nil, err
		}
		b, err := betafus[info.Candidate].Result()
		if err != nil {
			return nil, err
		}
		log.Debug("get a,b", log.Dict{"a": a, "b": b, "key": s.Key(business_namespcae, target_namespacestring, info.Candidate)})
		result = append(result, &thompsonsampling_pb.CandidateUpdateInfo{
			Candidate: info.Candidate,
			Param: &thompsonsampling_pb.BetaParam{
				Alpha: a,
				Beta:  b,
			},
		})
	}
	return result, nil
}

// UpdateCandidate
func (s *Server) UpdateCandidate(ctx context.Context, in *thompsonsampling_pb.CandidateUpdateQuery) (*thompsonsampling_pb.CandidateUpdateResponse, error) {
	var result []*thompsonsampling_pb.CandidateUpdateInfo
	var err error

	Business := ""
	Target := ""
	if in.Scope != nil {
		Business = in.Scope.Business
		Target = in.Scope.Target
	}
	switch in.UpdateType {
	case thompsonsampling_pb.CandidateUpdateQuery_RESET:
		{
			if in.Ttl >= 1 {
				result, err = s.ResetParamToRedis(Business, Target, time.Duration(in.Ttl)*time.Second, in.CandidateUpdateInfo...)
			} else {
				result, err = s.ResetParamToRedis(Business, Target, time.Duration(s.DefaultKeyTTL)*time.Second, in.CandidateUpdateInfo...)
			}
		}
	default:
		{
			if in.Ttl >= 1 {
				result, err = s.IncrParamToRedis(Business, Target, time.Duration(in.Ttl)*time.Second, in.CandidateUpdateInfo...)
			} else {
				result, err = s.IncrParamToRedis(Business, Target, time.Duration(s.DefaultKeyTTL)*time.Second, in.CandidateUpdateInfo...)
			}
		}
	}
	if err != nil {
		return nil, err
	}
	return &thompsonsampling_pb.CandidateUpdateResponse{
		CandidateUpdateInfo: result,
	}, nil
}
