package thompsonsampling

import (
	"context"
	"sort"
	"sync"

	log "github.com/Golang-Tools/loggerhelper"
	rp "github.com/Golang-Tools/redishelper/proxy"
	"github.com/go-redis/redis/v8"
	"gonum.org/v1/gonum/stat/distuv"
)

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

type WeightedCandidate struct {
	Candidate string
	Weight    float64
}

type WeightedCandidateSlice []*WeightedCandidate

func (s WeightedCandidateSlice) Len() int { return len(s) }

func (s WeightedCandidateSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

func (s WeightedCandidateSlice) Less(i, j int) bool { return s[i].Weight < s[j].Weight }

//Rank
func (s *Server) Rank(ctx context.Context, in *RankQuery) (*RankResponse, error) {
	log.Info("Rank get message ", log.Dict{"in": in})
	params, err := s.getParamFromRedis(in.BusinessNamespcae, in.TargetNamespace, in.Candidates...)
	if err != nil {
		return nil, err
	}
	z := make([]*WeightedCandidate, len(in.Candidates))
	wg := sync.WaitGroup{}
	for i, candidate := range in.Candidates {
		wg.Add(1)
		func(candidate string, p *BetaParam) {
			defer wg.Done()
			dist := distuv.Beta{
				Alpha: p.Alpha + 1,
				Beta:  p.Beta + 1,
			}
			z[i] = &WeightedCandidate{
				Candidate: candidate,
				Weight:    dist.Rand(),
			}
		}(candidate, params[i])
	}
	wg.Wait()
	if in.Desc {
		sort.Sort(sort.Reverse(WeightedCandidateSlice(z)))
	} else {
		sort.Sort(WeightedCandidateSlice(z))
	}

	log.Info("result", log.Dict{"z": z})
	ordered_candidates := []string{}
	for _, wc := range z {
		ordered_candidates = append(ordered_candidates, wc.Candidate)
	}
	return &RankResponse{OrderedCandidates: ordered_candidates}, nil
}
