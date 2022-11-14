package thompsonsampling_serv

import (
	"context"
	"sort"
	"sync"

	"github.com/featmate/thompsonsampling-orderrpc/thompsonsampling_pb"
	"gonum.org/v1/gonum/stat/distuv"
)

// Rank
func (s *Server) Rank(ctx context.Context, in *thompsonsampling_pb.RankQuery) (*thompsonsampling_pb.RankResponse, error) {
	Business := ""
	Target := ""
	if in.Scope != nil {
		Business = in.Scope.Business
		Target = in.Scope.Target
	}
	params, err := s.getParamFromRedis(Business, Target, in.Candidates...)
	if err != nil {
		return nil, err
	}
	z := make([]*thompsonsampling_pb.WeightedCandidate, len(in.Candidates))
	wg := sync.WaitGroup{}
	for i, candidate := range in.Candidates {
		wg.Add(1)
		func(candidate string, p *thompsonsampling_pb.BetaParam) {
			defer wg.Done()
			if p == nil {
				p = &thompsonsampling_pb.BetaParam{
					Alpha: 0,
					Beta:  0,
				}
			}
			dist := s.betapool.Get().(*distuv.Beta)
			dist.Alpha = p.Alpha + 1
			dist.Beta = p.Beta + 1
			z[i] = &thompsonsampling_pb.WeightedCandidate{
				Candidate: candidate,
				Weight:    dist.Rand(),
			}
			s.betapool.Put(dist)
		}(candidate, params[candidate])
	}
	wg.Wait()
	if in.Desc {
		sort.Sort(sort.Reverse(WeightedCandidateSlice(z)))
	} else {
		sort.Sort(WeightedCandidateSlice(z))
	}
	return &thompsonsampling_pb.RankResponse{OrderedCandidates: z}, nil
}

// Top
func (s *Server) Top(ctx context.Context, in *thompsonsampling_pb.TopQuery) (*thompsonsampling_pb.TopResponse, error) {
	Business := ""
	Target := ""
	if in.Scope != nil {
		Business = in.Scope.Business
		Target = in.Scope.Target
	}
	params, err := s.getParamFromRedis(Business, Target, in.Candidates...)
	if err != nil {
		return nil, err
	}
	z := make([]*thompsonsampling_pb.WeightedCandidate, len(in.Candidates))
	wg := sync.WaitGroup{}
	for i, candidate := range in.Candidates {
		wg.Add(1)
		func(candidate string, p *thompsonsampling_pb.BetaParam) {
			defer wg.Done()
			if p == nil {
				p = &thompsonsampling_pb.BetaParam{
					Alpha: 0,
					Beta:  0,
				}
			}
			dist := s.betapool.Get().(*distuv.Beta)
			dist.Alpha = p.Alpha + 1
			dist.Beta = p.Beta + 1
			z[i] = &thompsonsampling_pb.WeightedCandidate{
				Candidate: candidate,
				Weight:    dist.Rand(),
			}
			s.betapool.Put(dist)
		}(candidate, params[candidate])
	}
	wg.Wait()
	var maxWeight float64 = 0
	var maxWeightCanditate = ""

	for _, c := range z {
		if c.Weight > maxWeight {
			maxWeightCanditate = c.Candidate
			maxWeight = c.Weight
		}
	}
	return &thompsonsampling_pb.TopResponse{Candidate: maxWeightCanditate}, nil
}
