package grpc

import (
	"context"
	"testing"

	grpclib "google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestPSKInterceptor_ValidToken(t *testing.T) {
	interceptor := NewPSKInterceptor("test-token")
	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("authorization", "Bearer test-token"))
	_, err := interceptor.Unary(ctx, nil, &grpclib.UnaryServerInfo{}, func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
}

func TestPSKInterceptor_InvalidToken(t *testing.T) {
	interceptor := NewPSKInterceptor("test-token")
	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("authorization", "Bearer wrong"))
	_, err := interceptor.Unary(ctx, nil, &grpclib.UnaryServerInfo{}, func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	})
	if err == nil {
		t.Fatal("invalid token should be rejected")
	}
}

func TestPSKInterceptor_MissingToken(t *testing.T) {
	interceptor := NewPSKInterceptor("test-token")
	_, err := interceptor.Unary(context.Background(), nil, &grpclib.UnaryServerInfo{}, func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	})
	if err == nil {
		t.Fatal("missing token should be rejected")
	}
}
