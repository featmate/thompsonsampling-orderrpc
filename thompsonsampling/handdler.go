package thompsonsampling

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	log "github.com/Golang-Tools/loggerhelper"
	rp "github.com/Golang-Tools/redishelper/proxy"
	"github.com/go-redis/redis/v8"
	"gonum.org/v1/gonum/stat/distuv"
)

const ALGONamespace = "Tompsonsampling::"

func (s *Server) queryRedisCtx() (context.Context, context.CancelFunc) {
	var ctx context.Context
	var cancel context.CancelFunc
	if s.QueryRedisTimeout > 0 {
		timeout := time.Duration(s.QueryRedisTimeout) * time.Millisecond
		ctx, cancel = context.WithTimeout(context.Background(), timeout)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	return ctx, cancel
}

func buildKey(business_namespcae, target_namespacestring, candidate string) string {
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

func (s *Server) queryRedis(business_namespcae, target_namespacestring string, candidates ...string) ([]*BetaParam, error) {
	ctx, cancel := s.queryRedisCtx()
	defer cancel()
	pipe := rp.Proxy.TxPipeline()
	futures := []*redis.SliceCmd{}
	for _, candidate := range candidates {
		key := buildKey(business_namespcae, target_namespacestring, candidate)
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
func (s *Server) Rank(ctx context.Context, in *Query) (*Response, error) {
	log.Info("Rank get message ", log.Dict{"in": in})
	params, err := s.queryRedis(in.BusinessNamespcae, in.TargetNamespace, in.Candidates...)
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
				Alpha: p.Alpha,
				Beta:  p.Beta,
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

	// m := &Response{OrderedCandidates: math.Pow(in.Message, 2)}
	log.Info("result", log.Dict{"z": z})
	ordered_candidates := []string{}
	for _, wc := range z {
		ordered_candidates = append(ordered_candidates, wc.Candidate)
	}
	return &Response{OrderedCandidates: ordered_candidates}, nil
}
