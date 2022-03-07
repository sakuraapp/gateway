package server

import (
	"context"
	"fmt"
	gatewaypb "github.com/sakuraapp/protobuf/gateway"
	"github.com/sakuraapp/shared/pkg/model"
	"github.com/sakuraapp/shared/pkg/resource"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"net"
	"time"
)

const maxConnectionAge = 5 * time.Minute

func (s *Server) SetCurrentItem(ctx context.Context, req *gatewaypb.SetCurrentItemRequest) (*gatewaypb.SetCurrentItemResponse, error) {
	i := req.Item

	roomId := model.RoomId(req.RoomId)
	item := &resource.MediaItem{
		Id:     i.Id,
		Author: model.UserId(i.Author),
		Type:   resource.MediaItemType(i.Type),
		MediaItemInfo: &resource.MediaItemInfo{
			Title: i.Title,
			Icon: i.Icon,
			Url: i.Url,
		},
	}

	err := s.handlers.SetCurrentItem(ctx, roomId, item)

	if err != nil {
		return nil, err
	}

	return &gatewaypb.SetCurrentItemResponse{}, nil
}

func (s *Server) initGrpc() {
	creds, err := credentials.NewServerTLSFromFile(s.GrpcCertPath, s.GrpcKeyPath)

	if err != nil {
		log.WithError(err).Fatal("Failed to load gRPC SSL/TLS key pair")
	}

	// gRPC & the websocket server run on different ports because the websocket server is meant to be public while the gRPC server is an internal service
	addr := fmt.Sprintf("0.0.0.0:%v", s.GrpcPort)
	listener, err := net.Listen("tcp", addr)

	if err != nil {
		log.WithError(err).Fatal("Failed to start gRPC TCP server")
	}

	log.Printf("gRPC Listening on port %v", s.GrpcPort)

	opts := []grpc.ServerOption{
		grpc.Creds(creds),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionAge: maxConnectionAge,
		}),
	}

	grpcServer := grpc.NewServer(opts...)
	s.grpc = grpcServer

	gatewaypb.RegisterGatewayServiceServer(grpcServer, s)
	err = grpcServer.Serve(listener)

	if err != nil {
		log.WithError(err).Fatal("Failed to start gRPC server")
	}
}