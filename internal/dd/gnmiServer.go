/*
Copyright 2021 Wim Henderickx.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package dd

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"time"

	config "github.com/netw-device-driver/ndd-grpc/config/configpb"
	"github.com/netw-device-driver/ndd-runtime/pkg/logging"
	"github.com/openconfig/gnmi/coalesce"
	"github.com/openconfig/gnmi/ctree"
	"github.com/openconfig/gnmi/path"
	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/openconfig/gnmi/subscribe"
	"github.com/pkg/errors"
	"golang.org/x/sync/semaphore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

const (
	defaultMaxSubscriptions = 64
	defaultMaxGetRPC        = 64
)

type streamClient struct {
	target  string
	req     *gnmi.SubscribeRequest
	queue   *coalesce.Queue
	stream  gnmi.GNMI_SubscribeServer
	errChan chan<- error
}

type Config struct {
	// Address
	GrpcServerAddress string
	// Generic
	MaxSubscriptions int64
	MaxUnaryRPC      int64
	// TLS
	InSecure   bool
	SkipVerify bool
	CaFile     string
	CertFile   string
	KeyFile    string
	// observability
	EnableMetrics bool
	Debug         bool
}

// Option can be used to manipulate Options.
type ServerOption func(*Server)

// WithLogger specifies how the Reconciler should log messages.
func WithServerLogger(log logging.Logger) ServerOption {
	return func(g *Server) {
		g.log = log
	}
}

func WithServerConfig(cfg Config) ServerOption {
	return func(g *Server) {
		g.cfg = cfg
	}
}

type Server struct {
	gnmi.UnimplementedGNMIServer
	cfg Config

	//m               *match.Match
	//c               *cache.Cache // cache to serve updates to k8s resource, dummy cache to handle gnmi notifications
	Cache *Cache //Cache that is handling k8s resources
	//Regi  *Register
	subscribeRPCsem *semaphore.Weighted
	unaryRPCsem     *semaphore.Weighted
	log             logging.Logger
}

func NewServer(target string, opts ...ServerOption) *Server {
	g := &Server{
		//m: match.New(),
		//c: cache.New([]string{target}),
	}

	for _, opt := range opts {
		opt(g)
	}

	// initialize register
	//g.Register = NewRegister(
	//	WithRegisterLogger(g.log),
	//)
	// initialize the k8s Cache
	g.Cache = NewCache(target,
		//WithGNMICache(g.c),
		WithCacheLogger(g.log),
		WithParser(g.log),
	)

	//g.c.SetClient(g.Update)

	return g
}

/*
func (s *Server) Update(n *ctree.Leaf) {
	switch v := n.Value().(type) {
	case *gnmi.Notification:
		subscribe.UpdateNotification(s.m, n, v, path.ToStrings(v.Prefix, true))
	default:
		s.log.Debug("unexpected update type", "type", v)
	}
}
*/

func (s *Server) Run(ctx context.Context) error {
	s.log.Debug("grpc server run...")
	errChannel := make(chan error)
	go func() {
		if err := s.Start(); err != nil {
			errChannel <- errors.Wrap(err, errStartGRPCServer)
		}
		errChannel <- nil
	}()
	return nil
	//return <-errChannel
}

// Start GRPC Server
func (s *Server) Start() error {
	s.subscribeRPCsem = semaphore.NewWeighted(defaultMaxSubscriptions)
	s.unaryRPCsem = semaphore.NewWeighted(defaultMaxGetRPC)
	s.log.Debug("grpc server start...")

	s.Cache.c.SetClient(s.Cache.UpdateGNMICache)
	s.log.Debug("gnmi cache set client...")
	s.Cache.UpdateResourceInGNMICache("dummy")
	s.log.Debug("gnmi cache UpdateResourceInGNMICache dummy...")

	// create a listener on a specific address:port
	l, err := net.Listen("tcp", s.cfg.GrpcServerAddress)
	if err != nil {
		return errors.Wrap(err, errCreateTcpListener)
	}

	// TODO, proper handling of the certificates with CERT Manager
	/*
		opts, err := s.serverOpts()
		if err != nil {
			return err
		}
	*/
	// create a gRPC server object
	//grpcServer := grpc.NewServer(opts...)
	grpcServer := grpc.NewServer()

	// attach the register service to the grpc server
	//register.RegisterRegistrationServer(grpcServer, s.Register)
	config.RegisterConfigurationServer(grpcServer, s.Cache)

	// attach the gnmi service to the grpc server
	gnmi.RegisterGNMIServer(grpcServer, s)

	// start the server
	s.log.Debug("grpc server serve...")
	if err := grpcServer.Serve(l); err != nil {
		s.log.Debug("Errors", "error", err)
		return errors.Wrap(err, errGrpcServer)
	}
	return nil
}

/*
func (s *Server) serverOpts() ([]grpc.ServerOption, error) {
	opts := make([]grpc.ServerOption, 0)
	if s.cfg.EnableMetrics {
		opts = append(opts, grpc.StreamInterceptor(grpc_prometheus.StreamServerInterceptor))
	}
	if s.cfg.SkipVerify || s.cfg.CaFile != "" || (s.cfg.CertFile != "" && s.cfg.KeyFile != "") {
		tlscfg := &tls.Config{
			Renegotiation:      tls.RenegotiateNever,
			InsecureSkipVerify: s.cfg.SkipVerify,
		}
		if s.cfg.CertFile != "" && s.cfg.KeyFile != "" {
			certificate, err := tls.LoadX509KeyPair(s.cfg.CertFile, s.cfg.KeyFile)
			if err != nil {
				return nil, err
			}
			tlscfg.Certificates = []tls.Certificate{certificate}
			// tlscfg.BuildNameToCertificate()
		} else {
			cert, err := SelfSignedCerts()
			if err != nil {
				return nil, err
			}
			tlscfg.Certificates = []tls.Certificate{cert}
		}
		if s.cfg.CaFile != "" {
			certPool := x509.NewCertPool()
			caFile, err := ioutil.ReadFile(s.cfg.CaFile)
			if err != nil {
				return nil, err
			}
			if ok := certPool.AppendCertsFromPEM(caFile); !ok {
				return nil, errors.New("failed to append certificate")
			}
			tlscfg.RootCAs = certPool
		}
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlscfg)))
	}
	return opts, nil
}
*/

func SelfSignedCerts() (tls.Certificate, error) {
	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * time.Hour)

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, nil
	}
	certTemplate := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"ndd.yndd.io"},
		},
		DNSNames:              []string{"ndd.yndd.io"},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	priv, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return tls.Certificate{}, nil
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, certTemplate, certTemplate, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, nil
	}
	certBuff := new(bytes.Buffer)
	keyBuff := new(bytes.Buffer)
	pem.Encode(certBuff, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	pem.Encode(keyBuff, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	return tls.X509KeyPair(certBuff.Bytes(), keyBuff.Bytes())
}

func (s *Server) handleSubscriptionRequest(sc *streamClient) {
	var err error
	s.log.Debug("processing subscription", "Target", sc.target)
	defer func() {
		if err != nil {
			s.log.Debug("error processing subscription", "Target", sc.target, "Error", err)
			sc.queue.Close()
			sc.errChan <- err
			return
		}
		s.log.Debug("subscription request processed", "Target", sc.target)
	}()

	if !sc.req.GetSubscribe().GetUpdatesOnly() {
		for _, sub := range sc.req.GetSubscribe().GetSubscription() {
			var fp []string
			fp, err = path.CompletePath(sc.req.GetSubscribe().GetPrefix(), sub.GetPath())
			if err != nil {
				return
			}
			err = s.Cache.c.Query(sc.target, fp,
				func(_ []string, l *ctree.Leaf, _ interface{}) error {
					if err != nil {
						return err
					}
					_, err = sc.queue.Insert(l)
					return nil
				})
			if err != nil {
				s.log.Debug("failed internal cache query", "Target", sc.target, "Error", err)
				return
			}
		}
	}
	_, err = sc.queue.Insert(syncMarker{})
}

func (s *Server) sendStreamingResults(sc *streamClient) {
	ctx := sc.stream.Context()
	peer, _ := peer.FromContext(ctx)
	s.log.Debug("sending streaming results", "Target", sc.target, "Peer", peer.Addr)
	defer s.subscribeRPCsem.Release(1)
	for {
		item, dup, err := sc.queue.Next(ctx)
		if coalesce.IsClosedQueue(err) {
			sc.errChan <- nil
			return
		}
		if err != nil {
			sc.errChan <- err
			return
		}
		if _, ok := item.(syncMarker); ok {
			err = sc.stream.Send(&gnmi.SubscribeResponse{
				Response: &gnmi.SubscribeResponse_SyncResponse{
					SyncResponse: true,
				}})
			if err != nil {
				sc.errChan <- err
				return
			}
			continue
		}
		node, ok := item.(*ctree.Leaf)
		if !ok || node == nil {
			sc.errChan <- status.Errorf(codes.Internal, "invalid cache node: %+v", item)
			return
		}
		err = s.sendSubscribeResponse(&resp{
			stream: sc.stream,
			n:      node,
			dup:    dup,
		}, sc)
		if err != nil {
			s.log.Debug("failed sending subscribeResponse", "target", sc.target, "error", err)
			sc.errChan <- err
			return
		}
		// TODO: check if target was deleted ? necessary ?
	}
}

type resp struct {
	stream gnmi.GNMI_SubscribeServer
	n      *ctree.Leaf
	dup    uint32
}

func (s *Server) sendSubscribeResponse(r *resp, sc *streamClient) error {
	notif, err := subscribe.MakeSubscribeResponse(r.n.Value(), r.dup)
	if err != nil {
		return status.Errorf(codes.Unknown, "unknown error: %v", err)
	}
	// No acls
	return r.stream.Send(notif)

}
