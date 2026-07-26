package classifier

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "opensearch/gen/go/models"
)

// validClasses is the set of intent classes the model service can return.
// Any value outside this set is rejected before a gRPC call is made.
var validClasses = map[string]bool{
	"news": true,
	"factual": true,
	"code": true,
	"research": true,
	"commercial": true,
	"general": true,
}

// Intent is the result of classifying a query.
type Intent struct {
	Class string
	Confidence float32
	Uncertain bool
}

// Classifier is a gRPC client to the model service rpc Classify endpoint.
// It holds no model and does no inference — it sends queries over the wire
// and returns structured Intent values.
type Classifier struct {
	client pb.ModelServiceClient
	threshhold float32
}

// New creates a Classifier from an existing gRPC connection.
// The connection is created once in main.go and shared.
func New(conn *grpc.ClientConn, confidenceThreshold float32) *Classifier {
	 return  &Classifier{
		client: pb.NewModelServiceClient(conn),
		threshhold: confidenceThreshold,
	 }
}

// Classify sends the query to the model service and returns the detected intent.
// If confidence is below the threshold the Intent is marked uncertain —
// the orchestrator responds by hedging across the top two candidate engine sets.
func (c *Classifier) Classify(ctx context.Context, query string) (Intent, error){
	if query == "" {
		return Intent{}, fmt.Errorf("Classify: query  must not be empty")
	}

	resp, err := c.client.Classify(ctx, &pb.ClassifyRequest{Query:query})
	if err != nil {
		return Intent{}, fmt.Errorf("classify rpc: %w", err)

	}
	return Intent{
		Class: resp.Intent,
		Confidence: resp.Confidence,
		Uncertain: resp.Confidence< c.threshhold,		
	}, nil
}

// AgentIntent is called when the agent provides an intent in the request body.
// It validates the class against the known set without calling the model.
// Returns an error if the class is unknown — the agent provided an invalid value.
func (c *Classifier) AgentIntent(class string) (Intent, error) {
	if !validClasses[class] {
		return Intent{}, fmt.Errorf("unkown intent class %q",class)
	}
	return Intent{
		Class: class,
		Confidence: 1.0,
		Uncertain: false,
	}, nil
}

// IsModelServiceError returns true when the error originated from the gRPC
// model service rather than from local validation. Callers use this to
// distinguish between a bad request and a service availability problem.
func IsModelServiceError(err error) bool {
	if err == nil {
		return false
	}
	s, ok := status.FromError(err)
	if !ok {
		return false
	}
	return s.Code() !=codes.OK
}