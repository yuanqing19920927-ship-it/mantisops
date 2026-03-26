package grpc

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type PSKInterceptor struct {
	token string
}

func NewPSKInterceptor(token string) *PSKInterceptor {
	return &PSKInterceptor{token: token}
}

func (p *PSKInterceptor) Unary(ctx context.Context, req interface{},
	info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	if err := p.validate(ctx); err != nil {
		return nil, err
	}
	return handler(ctx, req)
}

func (p *PSKInterceptor) validate(ctx context.Context) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "missing metadata")
	}
	values := md.Get("authorization")
	if len(values) == 0 {
		return status.Error(codes.Unauthenticated, "missing authorization")
	}
	token := strings.TrimPrefix(values[0], "Bearer ")
	if token != p.token {
		return status.Error(codes.Unauthenticated, "invalid token")
	}
	return nil
}
