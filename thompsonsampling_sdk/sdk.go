package thompsonsampling_sdk

import (
	"github.com/Golang-Tools/grpcsdk"
	"github.com/featmate/thompsonsampling-orderrpc/thompsonsampling_pb"
)

type Conf grpcsdk.SDKConfig

var SDK = grpcsdk.New(thompsonsampling_pb.NewTHOMPSONSAMPLINGClient, &thompsonsampling_pb.THOMPSONSAMPLING_ServiceDesc)
