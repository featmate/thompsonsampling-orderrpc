package thompsonsampling_serv

import (
	log "github.com/Golang-Tools/loggerhelper/v2"
	"github.com/Golang-Tools/namespace"
	"github.com/Golang-Tools/redishelper/v2/redisproxy"
	"github.com/featmate/thompsonsampling-orderrpc/thompsonsampling_pb"
	"github.com/go-redis/redis/v8"
)

type WeightedCandidateSlice []*thompsonsampling_pb.WeightedCandidate

func (s WeightedCandidateSlice) Len() int { return len(s) }

func (s WeightedCandidateSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

func (s WeightedCandidateSlice) Less(i, j int) bool { return s[i].Weight < s[j].Weight }

func (s *Server) Key(business_namespcae, target_namespace, candidate string) string {
	if business_namespcae == "" {
		business_namespcae = s.Namespace_DefaultBusinessName
	}
	if target_namespace == "" {
		target_namespace = s.Namespace_DefaultTargetName
	}
	ns := namespace.NameSpcae{s.App_Name, s.Namespace_ALGO, business_namespcae, target_namespace}
	key := ns.FullName(candidate, namespace.WithRedisStyle())
	return key
}

func (s *Server) InfoKeySpace(business_namespcae, target_namespace string) string {
	if business_namespcae == "" {
		business_namespcae = s.Namespace_DefaultBusinessName
	}
	if target_namespace == "" {
		target_namespace = s.Namespace_DefaultTargetName
	}

	ns := namespace.NameSpcae{s.App_Name, s.Namespace_ALGOMeta, business_namespcae}
	key := ns.FullName(target_namespace, namespace.WithEtcdStyle()) + "/"
	return key
}
func (s *Server) InfoKey(business_namespcae, target_namespace, candidate string) string {
	if business_namespcae == "" {
		business_namespcae = s.Namespace_DefaultBusinessName
	}
	if target_namespace == "" {
		target_namespace = s.Namespace_DefaultTargetName
	}

	ns := namespace.NameSpcae{s.App_Name, s.Namespace_ALGOMeta, business_namespcae, target_namespace}
	key := ns.FullName(candidate, namespace.WithEtcdStyle())
	return key
}

func (s *Server) getParamFromRedis(business_namespcae, target_namespacestring string, candidates ...string) (map[string]*thompsonsampling_pb.BetaParam, error) {
	ctx, cancel := redisproxy.Default.NewCtx()
	defer cancel()
	pipe := redisproxy.Default.TxPipeline()
	futures := map[string]*redis.SliceCmd{}
	for _, candidate := range candidates {
		key := s.Key(business_namespcae, target_namespacestring, candidate)
		f := pipe.HMGet(ctx, key, "alpha", "beta")
		futures[candidate] = f
	}
	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, err
	}
	res := map[string]*thompsonsampling_pb.BetaParam{}
	for candidate, f := range futures {
		d := thompsonsampling_pb.BetaParam{}
		err := f.Scan(&d)
		if err != nil {
			log.Warn("scan value error", log.Dict{
				"err":                    err.Error(),
				"business_namespcae":     business_namespcae,
				"target_namespacestring": target_namespacestring,
				"candidate":              candidate,
				"value":                  f.Val(),
			})
		}
		res[candidate] = &d
	}
	return res, nil
}

func (s *Server) getTTLFromRedis(business_namespcae, target_namespacestring string, candidates ...string) (map[string]int64, error) {
	ctx, cancel := redisproxy.Default.NewCtx()
	defer cancel()
	pipe := redisproxy.Default.TxPipeline()
	futures := map[string]*redis.DurationCmd{}
	for _, candidate := range candidates {
		key := s.Key(business_namespcae, target_namespacestring, candidate)
		f := pipe.TTL(ctx, key)
		futures[candidate] = f
	}
	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, err
	}
	res := map[string]int64{}
	for candidate, f := range futures {
		t, err := f.Result()
		if err != nil {
			log.Error("Result value error", log.Dict{
				"err":                    err.Error(),
				"business_namespcae":     business_namespcae,
				"target_namespacestring": target_namespacestring,
				"candidate":              candidate,
				"value":                  f.Val(),
			})
		}
		res[candidate] = int64(t.Seconds())
	}
	return res, nil
}
