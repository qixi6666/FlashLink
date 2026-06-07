package grpcapi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/jd/flashlink/internal/app/linkapp"
	"github.com/jd/flashlink/internal/app/statapp"
	"github.com/jd/flashlink/internal/domain/link"
	"github.com/jd/flashlink/internal/interfaces/grpcapi/pb"
	"google.golang.org/grpc"
	_ "google.golang.org/grpc/balancer/roundrobin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	ServiceLink     = "linksvc"
	ServiceRedirect = "redirectsvc"
	ServiceStats    = "statsvc"

	defaultCallTimeout = 2 * time.Second
)

type GatewayClient struct {
	linkConn       *grpc.ClientConn
	redirectConn   *grpc.ClientConn
	statsConn      *grpc.ClientConn
	linkClient     pb.LinkServiceClient
	redirectClient pb.RedirectServiceClient
	statsClient    pb.StatServiceClient
	timeout        time.Duration
}

func NewGatewayClient(ctx context.Context, linkAddr string, redirectAddr string, statsAddr string) (*GatewayClient, error) {
	linkConn, err := dial(ctx, linkAddr)
	if err != nil {
		return nil, err
	}
	redirectConn, err := dial(ctx, redirectAddr)
	if err != nil {
		_ = linkConn.Close()
		return nil, err
	}
	statsConn, err := dial(ctx, statsAddr)
	if err != nil {
		_ = linkConn.Close()
		_ = redirectConn.Close()
		return nil, err
	}

	return &GatewayClient{
		linkConn:       linkConn,
		redirectConn:   redirectConn,
		statsConn:      statsConn,
		linkClient:     pb.NewLinkServiceClient(linkConn),
		redirectClient: pb.NewRedirectServiceClient(redirectConn),
		statsClient:    pb.NewStatServiceClient(statsConn),
		timeout:        defaultCallTimeout,
	}, nil
}

func (c *GatewayClient) Close() error {
	var err error
	if c.linkConn != nil {
		err = errors.Join(err, c.linkConn.Close())
	}
	if c.redirectConn != nil {
		err = errors.Join(err, c.redirectConn.Close())
	}
	if c.statsConn != nil {
		err = errors.Join(err, c.statsConn.Close())
	}
	return err
}

func (c *GatewayClient) CreateShortLink(ctx context.Context, req linkapp.CreateRequest) (linkapp.CreateResponse, error) {
	callCtx, cancel := c.withTimeout(ctx)
	defer cancel()

	resp, err := c.linkClient.CreateShortLink(callCtx, &pb.CreateShortLinkRequest{
		LongUrl:  req.LongURL,
		Domain:   req.Domain,
		ExpireAt: toTimestamp(req.ExpireAt),
	})
	if err != nil {
		return linkapp.CreateResponse{}, fromStatus(err)
	}

	return linkapp.CreateResponse{
		Code:     resp.Code,
		ShortURL: resp.ShortUrl,
		LongURL:  resp.LongUrl,
		ExpireAt: fromTimestampPtr(resp.ExpireAt),
	}, nil
}

func (c *GatewayClient) Resolve(ctx context.Context, code string) (link.ShortLink, error) {
	callCtx, cancel := c.withTimeout(ctx)
	defer cancel()

	resp, err := c.redirectClient.Resolve(callCtx, &pb.ResolveRequest{Code: code})
	if err != nil {
		return link.ShortLink{}, fromStatus(err)
	}
	return fromShortLinkMessage(resp.Item), nil
}

func (c *GatewayClient) GetLinkStats(ctx context.Context, code string) (link.LinkStats, error) {
	callCtx, cancel := c.withTimeout(ctx)
	defer cancel()

	resp, err := c.statsClient.GetLinkStats(callCtx, &pb.LinkStatsRequest{Code: code})
	if err != nil {
		return link.LinkStats{}, fromStatus(err)
	}
	return fromLinkStatsResponse(resp), nil
}

func (c *GatewayClient) Record(ctx context.Context, event statapp.VisitEvent) error {
	callCtx, cancel := c.withTimeout(ctx)
	defer cancel()

	_, err := c.statsClient.RecordVisit(callCtx, &pb.RecordVisitRequest{
		Code:      event.Code,
		Ip:        event.IP,
		UserAgent: event.UserAgent,
		Referer:   event.Referer,
		VisitedAt: timestamppb.New(event.VisitedAt),
	})
	return fromStatus(err)
}

func (c *GatewayClient) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := c.timeout
	if timeout <= 0 {
		timeout = defaultCallTimeout
	}
	return context.WithTimeout(ctx, timeout)
}

func dial(ctx context.Context, addr string) (*grpc.ClientConn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	return grpc.DialContext(
		dialCtx,
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
		grpc.WithBlock(),
	)
}

func NewServer() *grpc.Server {
	server := grpc.NewServer()
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(server, healthServer)
	return server
}

func Serve(ctx context.Context, server *grpc.Server, listener net.Listener) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(listener)
	}()

	select {
	case <-ctx.Done():
		stopped := make(chan struct{})
		go func() {
			server.GracefulStop()
			close(stopped)
		}()
		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			server.Stop()
		}
		return nil
	case err := <-errCh:
		return err
	}
}

func RegisterLinkService(server *grpc.Server, service *linkapp.Service) {
	pb.RegisterLinkServiceServer(server, &linkServiceServer{service: service})
}

func RegisterRedirectService(server *grpc.Server, service *linkapp.Service) {
	pb.RegisterRedirectServiceServer(server, &redirectServiceServer{service: service})
}

func RegisterStatService(server *grpc.Server, service *statapp.Service, recorder *statapp.Recorder) {
	pb.RegisterStatServiceServer(server, &statServiceServer{service: service, recorder: recorder})
}

type linkServiceServer struct {
	pb.UnimplementedLinkServiceServer
	service *linkapp.Service
}

func (s *linkServiceServer) CreateShortLink(ctx context.Context, req *pb.CreateShortLinkRequest) (*pb.CreateShortLinkResponse, error) {
	resp, err := s.service.CreateShortLink(ctx, linkapp.CreateRequest{
		LongURL:  req.LongUrl,
		Domain:   req.Domain,
		ExpireAt: fromTimestampPtr(req.ExpireAt),
	})
	if err != nil {
		return nil, toStatus(err)
	}
	return &pb.CreateShortLinkResponse{
		Code:     resp.Code,
		ShortUrl: resp.ShortURL,
		LongUrl:  resp.LongURL,
		ExpireAt: toTimestamp(resp.ExpireAt),
	}, nil
}

type redirectServiceServer struct {
	pb.UnimplementedRedirectServiceServer
	service *linkapp.Service
}

func (s *redirectServiceServer) Resolve(ctx context.Context, req *pb.ResolveRequest) (*pb.ResolveResponse, error) {
	resp, err := s.service.Resolve(ctx, req.Code)
	if err != nil {
		return nil, toStatus(err)
	}
	return &pb.ResolveResponse{Item: toShortLinkMessage(resp)}, nil
}

type statServiceServer struct {
	pb.UnimplementedStatServiceServer
	service  *statapp.Service
	recorder *statapp.Recorder
}

func (s *statServiceServer) GetLinkStats(ctx context.Context, req *pb.LinkStatsRequest) (*pb.LinkStatsResponse, error) {
	resp, err := s.service.GetLinkStats(ctx, req.Code)
	if err != nil {
		return nil, toStatus(err)
	}
	return toLinkStatsResponse(resp), nil
}

func (s *statServiceServer) RecordVisit(ctx context.Context, req *pb.RecordVisitRequest) (*pb.RecordVisitResponse, error) {
	err := s.recorder.Record(ctx, statapp.VisitEvent{
		Code:      req.Code,
		IP:        req.Ip,
		UserAgent: req.UserAgent,
		Referer:   req.Referer,
		VisitedAt: req.VisitedAt.AsTime(),
	})
	if err != nil {
		return nil, toStatus(err)
	}
	return &pb.RecordVisitResponse{}, nil
}

func toShortLinkMessage(item link.ShortLink) *pb.ShortLink {
	return &pb.ShortLink{
		Id:        item.ID,
		Code:      item.Code,
		LongUrl:   item.LongURL,
		Domain:    item.Domain,
		ExpireAt:  toTimestamp(item.ExpireAt),
		Status:    uint32(item.Status),
		CreatedAt: timestamppb.New(item.CreatedAt),
		UpdatedAt: timestamppb.New(item.UpdatedAt),
	}
}

func fromShortLinkMessage(item *pb.ShortLink) link.ShortLink {
	if item == nil {
		return link.ShortLink{}
	}
	return link.ShortLink{
		ID:        item.Id,
		Code:      item.Code,
		LongURL:   item.LongUrl,
		Domain:    item.Domain,
		ExpireAt:  fromTimestampPtr(item.ExpireAt),
		Status:    link.ShortLinkStatus(item.Status),
		CreatedAt: item.CreatedAt.AsTime(),
		UpdatedAt: item.UpdatedAt.AsTime(),
	}
}

func toLinkStatsResponse(stats link.LinkStats) *pb.LinkStatsResponse {
	referers := make([]*pb.RefererStat, 0, len(stats.Referers))
	for _, referer := range stats.Referers {
		referers = append(referers, &pb.RefererStat{
			Referer: referer.Referer,
			Pv:      referer.PV,
		})
	}
	return &pb.LinkStatsResponse{
		Code:      stats.Code,
		Pv:        stats.PV,
		Uv:        stats.UV,
		TodayPv:   stats.TodayPV,
		Referers:  referers,
		UpdatedAt: timestamppb.New(stats.UpdatedAt),
	}
}

func fromLinkStatsResponse(resp *pb.LinkStatsResponse) link.LinkStats {
	referers := make([]link.RefererStat, 0, len(resp.Referers))
	for _, referer := range resp.Referers {
		referers = append(referers, link.RefererStat{
			Referer: referer.Referer,
			PV:      referer.Pv,
		})
	}
	return link.LinkStats{
		Code:      resp.Code,
		PV:        resp.Pv,
		UV:        resp.Uv,
		TodayPV:   resp.TodayPv,
		Referers:  referers,
		UpdatedAt: resp.UpdatedAt.AsTime(),
	}
}

func toTimestamp(value *time.Time) *timestamppb.Timestamp {
	if value == nil {
		return nil
	}
	return timestamppb.New(*value)
}

func fromTimestampPtr(value *timestamppb.Timestamp) *time.Time {
	if value == nil {
		return nil
	}
	converted := value.AsTime()
	return &converted
}

func toStatus(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, link.ErrInvalidCode), errors.Is(err, link.ErrInvalidURL):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, link.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, link.ErrExpired):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, link.ErrQueueFull):
		return status.Error(codes.ResourceExhausted, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

func fromStatus(err error) error {
	if err == nil {
		return nil
	}
	st, ok := status.FromError(err)
	if !ok {
		return err
	}
	switch st.Code() {
	case codes.InvalidArgument:
		if strings.Contains(st.Message(), "url") {
			return link.ErrInvalidURL
		}
		return link.ErrInvalidCode
	case codes.NotFound:
		return link.ErrNotFound
	case codes.FailedPrecondition:
		return link.ErrExpired
	case codes.ResourceExhausted:
		return link.ErrQueueFull
	case codes.Unavailable:
		return fmt.Errorf("grpc service unavailable: %w", err)
	default:
		return err
	}
}
