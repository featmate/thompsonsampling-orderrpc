package thompsonsampling_serv

import (
	"context"
	"strings"

	"github.com/Golang-Tools/etcdhelper/etcdproxy"
	mapset "github.com/deckarep/golang-set/v2"
	"github.com/featmate/thompsonsampling-orderrpc/thompsonsampling_pb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// GetCandidateInfo
func (s *Server) GetScopeInfo(c context.Context, in *thompsonsampling_pb.Scope) (*thompsonsampling_pb.ScopeResponse, error) {
	if !s.ScopeObserveMode {
		return nil, ErrFeatureNotOn
	}
	ctx, cancel := etcdproxy.Default.NewCtx()
	defer cancel()
	prefix := s.InfoKeySpace(in.Business, in.Target)
	resp, err := etcdproxy.Default.KV.Get(ctx, prefix, clientv3.WithPrefix(), clientv3.WithSort(clientv3.SortByKey, clientv3.SortDescend))
	if err != nil {
		return nil, err
	}
	count := int64(0)
	res := map[string]int64{}
	for _, kv := range resp.Kvs {
		key := string(kv.Key)
		candidate := strings.ReplaceAll(key, prefix, "")
		sp, err := etcdproxy.Default.TimeToLive(ctx, clientv3.LeaseID(kv.Lease))
		if err != nil {
			return nil, err
		}
		res[candidate] = sp.TTL
		count += 1
	}
	return &thompsonsampling_pb.ScopeResponse{Count: count, List: res}, nil
}

// UpdateCandidate
func (s *Server) InScope(c context.Context, in *thompsonsampling_pb.InScopeQuery) (*thompsonsampling_pb.InScopeResponse, error) {
	if !s.ScopeObserveMode {
		return nil, ErrFeatureNotOn
	}
	ctx, cancel := etcdproxy.Default.NewCtx()
	defer cancel()
	Business := ""
	Target := ""
	if in.Scope != nil {
		Business = in.Scope.Business
		Target = in.Scope.Target
	}
	prefix := s.InfoKeySpace(Business, Target)
	resp, err := etcdproxy.Default.KV.Get(ctx, prefix, clientv3.WithPrefix(), clientv3.WithSort(clientv3.SortByKey, clientv3.SortDescend))
	if err != nil {
		return nil, err
	}
	checkcandidates := mapset.NewSet(in.Candidates...)
	allcandidates := mapset.NewSet[string]()
	for _, kv := range resp.Kvs {
		key := string(kv.Key)
		candidate := strings.ReplaceAll(key, prefix, "")
		allcandidates.Add(candidate)
	}
	result := map[string]bool{}
	ts := checkcandidates.Intersect(allcandidates)
	for _, candidate := range ts.ToSlice() {
		result[candidate] = true
	}
	fs := checkcandidates.Difference(ts)
	for _, candidate := range fs.ToSlice() {
		result[candidate] = false
	}
	return &thompsonsampling_pb.InScopeResponse{Result: result}, nil
}
