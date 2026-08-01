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
	RunnerUp string  // second best class, used by router for hedge routing
	Confidence float32
	Uncertain  bool
}

// Classifier is a gRPC client to the model service rpc Classify endpoint.
type Classifier struct {
	client pb.ModelServiceClient
	threshold float32
}

// New creates a Classifier from an existing gRPC connection.
func New(conn *grpc.ClientConn, confidenceThreshold float32) *Classifier {
	return &Classifier{
		client: pb.NewModelServiceClient(conn),
		threshold: confidenceThreshold,
	}
}

// Classify sends the query to the model service and returns the detected intent.
func (c *Classifier) Classify(ctx context.Context, query string) (Intent, error) {
	if query == "" {
		return Intent{}, fmt.Errorf("classify: query must not be empty")
	}

	resp, err := c.client.Classify(ctx, &pb.ClassifyRequest{Query: query})
	if err != nil {
		return Intent{}, fmt.Errorf("classify rpc: %w", err)
	}

	return Intent{
		Class: resp.Intent,
		RunnerUp: resp.RunnerUp,
		Confidence: resp.Confidence,
		Uncertain: resp.Confidence < c.threshold,
	}, nil
}

// AgentIntent validates an agent-provided intent class without calling the model.
func (c *Classifier) AgentIntent(class string) (Intent, error) {
	if !validClasses[class] {
		return Intent{}, fmt.Errorf("unknown intent class %q", class)
	}
	return Intent{
		Class: class,
		Confidence: 1.0,
		Uncertain: false,
	}, nil
}

// IsModelServiceError returns true when the error came from the gRPC model service.
func IsModelServiceError(err error) bool {
	if err == nil {
		return false
	}
	s, ok := status.FromError(err)
	if !ok {
		return false
	}
	return s.Code() != codes.OK
}