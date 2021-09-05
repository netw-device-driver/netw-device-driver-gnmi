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
	"time"

	"github.com/karimra/gnmic/target"
	"github.com/karimra/gnmic/types"
	ndrv1 "github.com/netw-device-driver/ndd-core/apis/dvr/v1"
	"google.golang.org/grpc"

	"github.com/netw-device-driver/ndd-runtime/pkg/logging"
	"github.com/netw-device-driver/ndd-runtime/pkg/utils"

	// initializes the devices with the _ import
	_ "github.com/netw-device-driver/netw-device-driver-gnmi/internal/devices/all"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// errors
	errConfigNtPresent  = "must specify Config"
	errCreateK8sClient  = "cannot create k8s client"
	errCreateGnmiClient = "cannot create gnmi client"
)

type Options struct {
	Scheme       *runtime.Scheme
	ServerConfig Config

	Namespace         string
	DeviceName        string
	TargetConfig      *types.TargetConfig
	Target            *target.Target
	CredName          string
	GrpcServerAddress string

	log   logging.Logger
	Debug bool
}

// Option can be used to manipulate Options.
type Option func(*Options)

// WithLogger specifies how the Reconciler should log messages.
func WithLogger(log logging.Logger) Option {
	return func(o *Options) {
		o.log = log
	}
}

// WithDeviceName initializes the device name in the device driver
func WithScheme(s *runtime.Scheme) Option {
	return func(o *Options) {
		o.Scheme = s
	}
}

// WithNameSpace initializes the namespace the device driver uses
func WithNameSpace(n *string) Option {
	return func(o *Options) {
		o.Namespace = *n
	}
}

// WithDeviceName initializes the device name in the device driver
func WithDeviceName(n *string) Option {
	return func(o *Options) {
		o.DeviceName = *n
	}
}

// WithGrpcServer initializes the grpc server in the device driver
func WithGrpcServer(s *string) Option {
	return func(o *Options) {
		o.GrpcServerAddress = *s
	}
}

func WithDebug(b *bool) Option {
	return func(o *Options) {
		o.Debug = *b
	}
}

// NewDeviceDriver creates a new device driver.
func NewDeviceDriver(config *rest.Config, opts ...Option) (*deviceDriver, error) {
	if config == nil {
		return nil, errors.New(errConfigNtPresent)
	}

	options := Options{}
	for _, opt := range opts {
		opt(&options)
	}

	//options = setOptionsDefaults(options)

	// creaate context
	ctx := context.Background()

	// get k8s client
	c, err := getClient(config, options)
	if err != nil {
		return nil, err
	}

	k8sopts := []K8sApiOption{
		WithK8sApiDeviceName(options.DeviceName),
		WithK8sApiNameSpace(options.Namespace),
		WithK8sApiClient(c),
		WithK8sApiLogger(options.log),
	}
	k8sApi := NewK8sApi(k8sopts...)

	// get network node
	nn, err := k8sApi.GetNetworkNode(ctx)
	if err != nil {
		return nil, err
	}

	options.CredName = nn.GetTargetCredentialsName()

	u, p, err := k8sApi.GetCredentials(ctx, options.CredName)
	if err != nil {
		return nil, err
	}
	options.TargetConfig = &types.TargetConfig{
		Name:       options.DeviceName,
		Address:    nn.GetTargetAddress(),
		Username:   u,
		Password:   p,
		Timeout:    10 * time.Second,
		SkipVerify: utils.BoolPtr(nn.GetTargetSkipVerify()),
		Insecure:   utils.BoolPtr(nn.GetTargetInsecure()),
		TLSCA:      utils.StringPtr(""), //TODO TLS
		TLSCert:    utils.StringPtr(""), //TODO TLS
		TLSKey:     utils.StringPtr(""), //TODO TLS
		Gzip:       utils.BoolPtr(false),
	}
	options.Target = target.NewTarget(options.TargetConfig)
	if err := options.Target.CreateGNMIClient(ctx, grpc.WithBlock()); err != nil { // TODO add dialopts
		return nil, errors.Wrap(err, errCreateGnmiClient)
	}

	options.ServerConfig = Config{
		GrpcServerAddress: options.GrpcServerAddress,
		SkipVerify:        true,
	}

	return &deviceDriver{
		ctx: ctx,
		//scheme:        options.Scheme,
		//client:        c,
		K8sApi:        k8sApi,
		DeviceName:    options.DeviceName,
		DeviceDetails: new(ndrv1.DeviceDetails),
		Server: NewServer(options.DeviceName,
			WithServerLogger(options.log),
			WithServerConfig(options.ServerConfig),
		),
		//Register: NewRegister(
		//	WithRegisterLogger(options.log),
		//),
		//Cache: NewCache(
		//	WithCacheLogger(options.log),
		//	WithParser(options.log),
		//	WithK8sClient(c),
		//),
		Target: NewTarget(options.Target,
			WithTargetLogger(options.log),
			WithTargetConfig(options.TargetConfig),
		),
		Collector: NewDeviceCollector(options.Target,
			WithDeviceCollectorLogger(options.log),
		),
		log: options.log,
	}, nil
}

// setOptionsDefaults set default values for Options fields.
//func setOptionsDefaults(options Options) Options {
//	return options
//}

// getClient gets the client to interact with the k8s apiserver
func getClient(config *rest.Config, options Options) (client.Client, error) {
	k8sclopts := client.Options{
		Scheme: options.Scheme,
	}
	c, err := client.New(config, k8sclopts)
	if err != nil {
		return nil, errors.Wrap(err, errCreateK8sClient)
	}
	return c, nil
}
