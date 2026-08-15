package crawler

import (
	"opensearch/internal/merger"

	"google.golang.org/grpc"
)

type Decision struct {
    Sufficient bool
    URLs []string
    EnrichedResults []merger.Result
}

type Request struct {
    Query string
    Intent string
    Results []merger.Result
}
type Decider struct {
	ModelConn  *grpc.ClientConn
	SpiderConn *grpc.ClientConn
}
