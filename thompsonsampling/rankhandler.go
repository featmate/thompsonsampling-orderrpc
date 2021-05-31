package thompsonsampling

import (
	"context"
	"sort"
	"sync"

	log "github.com/Golang-Tools/loggerhelper"
	"gonum.org/v1/gonum/stat/distuv"
)

//Rank
func (s *Server) Rank(ctx context.Context, in *RankQuery) (*RankResponse, error) {
	log.Info("Rank get message ", log.Dict{"in": in})
	params, err := s.getParamFromRedis(in.BusinessNamespace, in.TargetNamespace, in.Candidates...)
	if err != nil {
		return nil, err
	}
	z := make([]*WeightedCandidate, len(in.Candidates))
	wg := sync.WaitGroup{}
	for i, candidate := range in.Candidates {
		wg.Add(1)
		func(candidate string, p *BetaParam) {
			defer wg.Done()
			dist := s.betapool.Get().(*distuv.Beta)
			dist.Alpha = p.Alpha + 1
			dist.Beta = p.Beta + 1
			z[i] = &WeightedCandidate{
				Candidate: candidate,
				Weight:    dist.Rand(),
			}
			s.betapool.Put(dist)
		}(candidate, params[i])
	}
	wg.Wait()
	if in.Desc {
		sort.Sort(sort.Reverse(WeightedCandidateSlice(z)))
	} else {
		sort.Sort(WeightedCandidateSlice(z))
	}

	log.Info("result", log.Dict{"z": z})
	return &RankResponse{OrderedCandidates: z}, nil
}
