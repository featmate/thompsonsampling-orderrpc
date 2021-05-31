package thompsonsampling

import (
	"strings"

	log "github.com/Golang-Tools/loggerhelper"
	rp "github.com/Golang-Tools/redishelper/proxy"
	"github.com/go-redis/redis/v8"
)

const ALGONamespace = "Tompsonsampling::"

type WeightedCandidateSlice []*WeightedCandidate

func (s WeightedCandidateSlice) Len() int { return len(s) }

func (s WeightedCandidateSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

func (s WeightedCandidateSlice) Less(i, j int) bool { return s[i].Weight < s[j].Weight }

func BuildKey(business_namespcae, target_namespacestring, candidate string) string {
	if business_namespcae == "" {
		business_namespcae = "__global__"
	}
	if target_namespacestring == "" {
		target_namespacestring = "__global__"
	}
	builder := strings.Builder{}
	builder.Grow(len(ALGONamespace) + len(business_namespcae) + len(target_namespacestring) + len(candidate) + 4)
	builder.WriteString(ALGONamespace)
	builder.WriteString(business_namespcae)
	builder.WriteString("::")
	builder.WriteString(target_namespacestring)
	builder.WriteString("::")
	builder.WriteString(candidate)
	return builder.String()
}

type BetaParam struct {
	Alpha float64 `redis:"alpha"`
	Beta  float64 `redis:"beta"`
}

func (s *Server) getParamFromRedis(business_namespcae, target_namespacestring string, candidates ...string) ([]*BetaParam, error) {
	ctx, cancel := s.QueryRedisCtx()
	defer cancel()
	pipe := rp.Proxy.TxPipeline()
	futures := []*redis.SliceCmd{}
	for _, candidate := range candidates {
		key := BuildKey(business_namespcae, target_namespacestring, candidate)
		f := pipe.HMGet(ctx, key, "alpha", "beta")
		futures = append(futures, f)
	}
	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, err
	}
	res := []*BetaParam{}
	for index, f := range futures {
		d := BetaParam{}
		err := f.Scan(&d)
		if err != nil {
			log.Warn("scan value error", log.Dict{
				"err":                    err.Error(),
				"business_namespcae":     business_namespcae,
				"target_namespacestring": target_namespacestring,
				"candidate":              candidates[index],
				"value":                  f.Val(),
			})
		}
		res = append(res, &d)
	}
	return res, nil
}
