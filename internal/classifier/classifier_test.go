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

// mockModelService is an in-process gRPC server returning controlled responses.
// No real model, no real inference, no network outside the test process.
type mockModelService struct {
	pb.UnimplementedModelServiceServer
	intent  string
	confidence float32
	returnErr codes.Code
}

func (m *mockModelService) Classify(_ context.Context, req *pb.ClassifyRequest) (*pb.ClassifyResponse, error) {
	if m.returnErr != codes.OK {
		return nil, status.Error(m.returnErr, "mock error")
	}
	if req.Query == "" {
		return nil, status.Error(codes.InvalidArgument, "query must not be empty")
	}
	return &pb.ClassifyResponse{
		Intent:  m.intent,
		Confidence: m.confidence,
	}, nil
}

// startMockServer starts an in-process gRPC server and returns a Classifier
// connected to it. Server stops when the test ends.
func startMockServer(t *testing.T, mock *mockModelService, threshold float32) *Classifier {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
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
		t.Fatalf("failed to connect to mock server: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	return New(conn, threshold)
}

func TestClassifyReturnsCorrectIntent(t *testing.T) {
	clf := startMockServer(t, &mockModelService{
		intent: "code",
		confidence: 0.94,
	}, 0.65)

	intent, err := clf.Classify(context.Background(), "how to handle goroutine panic")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if intent.Class != "code" {
		t.Errorf("got class %q, want %q", intent.Class, "code")
	}
	if intent.Confidence != 0.94 {
		t.Errorf("got confidence %.2f, want 0.94", intent.Confidence)
	}
}

func TestClassifyMarksUncertainBelowThreshold(t *testing.T) {
	clf := startMockServer(t, &mockModelService{
		intent: "general",
		confidence: 0.55,
	}, 0.65)

	intent, err := clf.Classify(context.Background(), "something ambiguous")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !intent.Uncertain {
		t.Errorf("expected uncertain at confidence 0.55 with threshold 0.65")
	}
}

func TestClassifyConfidentAboveThreshold(t *testing.T) {
	clf := startMockServer(t, &mockModelService{
		intent: "news",
		confidence: 0.91,
	}, 0.65)

	intent, err := clf.Classify(context.Background(), "latest news today")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if intent.Uncertain {
		t.Errorf("expected confident at confidence 0.91 with threshold 0.65")
	}
}

func TestClassifyEmptyQueryReturnsError(t *testing.T) {
	clf := startMockServer(t, &mockModelService{
		intent: "general",
		confidence: 0.90,
	}, 0.65)

	_, err := clf.Classify(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty query, got nil")
	}
}

func TestClassifyModelServiceErrorPropagates(t *testing.T) {
	clf := startMockServer(t, &mockModelService{
		returnErr: codes.Unavailable,
	}, 0.65)

	_, err := clf.Classify(context.Background(), "some query")
	if err == nil {
		t.Error("expected error when model service unavailable, got nil")
	}
	if !IsModelServiceError(err) {
		t.Errorf("expected IsModelServiceError true, got false for: %v", err)
	}
}

func TestAgentIntentValidClass(t *testing.T) {
	clf := startMockServer(t, &mockModelService{}, 0.65)

	intent, err := clf.AgentIntent("research")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if intent.Class != "research" {
		t.Errorf("got class %q, want %q", intent.Class, "research")
	}
	if intent.Confidence != 1.0 {
		t.Errorf("agent intent confidence must be 1.0, got %.2f", intent.Confidence)
	}
	if intent.Uncertain {
		t.Error("agent intent must never be uncertain")
	}
}

func TestAgentIntentUnknownClassReturnsError(t *testing.T) {
	clf := startMockServer(t, &mockModelService{}, 0.65)

	_, err := clf.AgentIntent("invalid_class")
	if err == nil {
		t.Error("expected error for unknown intent class, got nil")
	}
}

func TestAgentIntentDoesNotCallModelService(t *testing.T) {
	// model service set to return Unavailable — if AgentIntent calls it
	// the error will surface and the test fails. It must not make a gRPC call.
	clf := startMockServer(t, &mockModelService{
		returnErr: codes.Unavailable,
	}, 0.65)

	_, err := clf.AgentIntent("code")
	if err != nil {
		t.Errorf("AgentIntent must not call model service, got: %v", err)
	}
}