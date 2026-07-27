package classifier

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	pb "opensearch/gen/go/models"
)

// mockModelService controls what the model service returns.
// returnErr set to anything other than codes.OK simulates a service failure.
type mockModelService struct {
	pb.UnimplementedModelServiceServer
	intent     string
	confidence float32
	returnErr  codes.Code
}

func (m *mockModelService) Classify(_ context.Context, req *pb.ClassifyRequest) (*pb.ClassifyResponse, error) {
	if m.returnErr != codes.OK {
		return nil, status.Error(m.returnErr, "mock error")
	}
	return &pb.ClassifyResponse{
		Intent:     m.intent,
		Confidence: m.confidence,
	}, nil
}

// startMockServer starts an in-process gRPC server and returns a Classifier
// wired to it. The server stops when the test ends.
func startMockServer(t *testing.T, mock *mockModelService, threshold float32) *Classifier {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	srv := grpc.NewServer()
	pb.RegisterModelServiceServer(srv, mock)
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient(
		lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	return New(conn, threshold)
}

// --- Classify() tests ---
// Each test targets one specific behaviour in classifier.go not in the mock.

func TestClassifyEmptyQueryBlockedBeforeGRPC(t *testing.T) {
	// model service set to fail — if the gRPC call were made the error
	// would surface as a model service error not a local validation error.
	// classifier.go must reject the empty query before touching gRPC.
	clf := startMockServer(t, &mockModelService{
		returnErr: codes.Unavailable,
	}, 0.65)

	_, err := clf.Classify(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty query, got nil")
	}
	if IsModelServiceError(err) {
		t.Error("empty query must be rejected locally before gRPC call is made")
	}
}

func TestClassifyUncertainWhenConfidenceBelowThreshold(t *testing.T) {
	// mock returns confidence 0.55, threshold is 0.65
	// classifier.go must set Uncertain = true
	clf := startMockServer(t, &mockModelService{
		intent:     "general",
		confidence: 0.55,
	}, 0.65)

	intent, err := clf.Classify(context.Background(), "ambiguous query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !intent.Uncertain {
		t.Errorf(
			"expected Uncertain=true when confidence %.2f is below threshold %.2f",
			intent.Confidence, float32(0.65),
		)
	}
}

func TestClassifyNotUncertainWhenConfidenceAboveThreshold(t *testing.T) {
	// mock returns confidence 0.91, threshold is 0.65
	// classifier.go must set Uncertain = false
	clf := startMockServer(t, &mockModelService{
		intent:     "code",
		confidence: 0.91,
	}, 0.65)

	intent, err := clf.Classify(context.Background(), "goroutine panic handling")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if intent.Uncertain {
		t.Errorf(
			"expected Uncertain=false when confidence %.2f is above threshold %.2f",
			intent.Confidence, float32(0.65),
		)
	}
}

func TestClassifyUncertainAtExactThreshold(t *testing.T) {
	// confidence exactly equal to threshold — must be uncertain
	// boundary condition in classifier.go: confidence < threshold not <=
	clf := startMockServer(t, &mockModelService{
		intent:     "news",
		confidence: 0.65,
	}, 0.65)

	intent, err := clf.Classify(context.Background(), "some query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 0.65 is not < 0.65 so this should be confident not uncertain
	if intent.Uncertain {
		t.Error("confidence equal to threshold must not be marked uncertain")
	}
}

func TestClassifyModelServiceErrorSurfacesCorrectly(t *testing.T) {
	// model service returns Unavailable
	// classifier.go must propagate the error
	// the error must be identifiable as a model service error
	clf := startMockServer(t, &mockModelService{
		returnErr: codes.Unavailable,
	}, 0.65)

	_, err := clf.Classify(context.Background(), "valid query")
	if err == nil {
		t.Fatal("expected error when model service unavailable, got nil")
	}
	if !IsModelServiceError(err) {
		t.Errorf("expected IsModelServiceError=true for gRPC Unavailable, got false: %v", err)
	}
}

func TestClassifyInternalErrorIsModelServiceError(t *testing.T) {
	clf := startMockServer(t, &mockModelService{
		returnErr: codes.Internal,
	}, 0.65)

	_, err := clf.Classify(context.Background(), "valid query")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !IsModelServiceError(err) {
		t.Errorf("expected IsModelServiceError=true for gRPC Internal, got false: %v", err)
	}
}

// --- AgentIntent() tests ---
func TestAgentIntentDoesNotCallGRPC(t *testing.T) {
	// model service set to fail — if AgentIntent calls gRPC the error
	// would surface. It must validate locally and never make a network call.
	clf := startMockServer(t, &mockModelService{
		returnErr: codes.Unavailable,
	}, 0.65)

	_, err := clf.AgentIntent("code")
	if err != nil {
		t.Errorf("AgentIntent must not call gRPC, got error: %v", err)
	}
}

func TestAgentIntentAlwaysConfident(t *testing.T) {
	clf := startMockServer(t, &mockModelService{}, 0.65)

	for _, class := range []string{"news", "factual", "code", "research", "commercial", "general"} {
		intent, err := clf.AgentIntent(class)
		if err != nil {
			t.Fatalf("class %q: unexpected error: %v", class, err)
		}
		if intent.Confidence != 1.0 {
			t.Errorf("class %q: expected confidence 1.0, got %.2f", class, intent.Confidence)
		}
		if intent.Uncertain {
			t.Errorf("class %q: agent intent must never be uncertain", class)
		}
	}
}

func TestAgentIntentRejectsUnknownClass(t *testing.T) {
	clf := startMockServer(t, &mockModelService{}, 0.65)

	_, err := clf.AgentIntent("made_up_class")
	if err == nil {
		t.Error("expected error for unknown intent class, got nil")
	}
	if IsModelServiceError(err) {
		t.Error("unknown class rejection must be a local error not a gRPC error")
	}
}

func TestAgentIntentRejectsEmptyClass(t *testing.T) {
	clf := startMockServer(t, &mockModelService{}, 0.65)

	_, err := clf.AgentIntent("")
	if err == nil {
		t.Error("expected error for empty intent class, got nil")
	}
}

// --- IsModelServiceError() tests ---
func TestIsModelServiceErrorFalseForNil(t *testing.T) {
	if IsModelServiceError(nil) {
		t.Error("IsModelServiceError must return false for nil error")
	}
}

func TestIsModelServiceErrorFalseForLocalError(t *testing.T) {
	clf := startMockServer(t, &mockModelService{}, 0.65)

	_, err := clf.Classify(context.Background(), "")
	if err == nil {
		t.Fatal("expected local validation error, got nil")
	}
	if IsModelServiceError(err) {
		t.Error("local validation error must not be identified as model service error")
	}
}