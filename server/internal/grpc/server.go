package grpc

import (
	"fmt"
	"log"
	"net"

	"google.golang.org/grpc"
	pb "mantisops/server/proto/gen"
)

func StartPlain(addr string, handler *Handler, psk *PSKInterceptor) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	s := grpc.NewServer(grpc.UnaryInterceptor(psk.Unary))
	pb.RegisterAgentServiceServer(s, handler)
	log.Printf("gRPC (plain+PSK) listening on %s", addr)
	return s.Serve(lis)
}
