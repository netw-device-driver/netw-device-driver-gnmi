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
	"context"
	"net"

	config "github.com/netw-device-driver/ndd-grpc/config/configpb"
	register "github.com/netw-device-driver/ndd-grpc/register/registerpb"
	"github.com/netw-device-driver/ndd-runtime/pkg/logging"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

const (
	errStartGRPCServer   = "cannot start grpc server"
	errGrpcServer        = "error grpc server"
	errCreateTcpListener = "cannot create tcp listener"
)

type GrpcServer struct {
	grpcServerAddress string
	log               logging.Logger
}

// Option can be used to manipulate Options.
type ServerOption func(*GrpcServer)

// WithLogger specifies how the Reconciler should log messages.
func WithServerLogger(log logging.Logger) ServerOption {
	return func(o *GrpcServer) {
		o.log = log
	}
}

func WithGrpcServerAddress(a string) ServerOption {
	return func(o *GrpcServer) {
		o.grpcServerAddress = a
	}
}

func NewGrpcServer(opts ...ServerOption) *GrpcServer {
	s := &GrpcServer{}
	for _, opt := range opts {
		opt(s)
	}

	return s
}

func (s *GrpcServer) GetAddress() string {
	return s.grpcServerAddress
}

func (s *GrpcServer) Run(ctx context.Context, r *Register, c *Cache) error {
	s.log.Debug("grpc server run...")
	errChannel := make(chan error)
	go func() {
		if err := s.Start(r, c); err != nil {
			errChannel <- errors.Wrap(err, errStartGRPCServer)
		}
		errChannel <- nil
	}()
	return nil
	//return <-errChannel
}

// StartGRPCServer starts the grpcs server
func (s *GrpcServer) Start(r *Register, c *Cache) error {
	s.log.Debug("grpc server start...")

	// create a listener on a specific address:port
	l, err := net.Listen("tcp", s.grpcServerAddress)
	if err != nil {
		return errors.Wrap(err, errCreateTcpListener)
	}

	// create a gRPC server object
	grpcServer := grpc.NewServer()

	// attach the gRPC service to the server
	register.RegisterRegistrationServer(grpcServer, r)
	config.RegisterConfigurationServer(grpcServer, c)

	// start the server
	s.log.Debug("grpc server serve...")
	if err := grpcServer.Serve(l); err != nil {
		s.log.Debug("Errors", "error", err)
		return errors.Wrap(err, errGrpcServer)
	}
	return nil
}
