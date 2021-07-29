module github.com/netw-device-driver/netw-device-driver-gnmi

go 1.16

require (
	github.com/google/go-cmp v0.5.6
	github.com/karimra/gnmic v0.17.1
	github.com/netw-device-driver/ndd-core v0.1.8
	github.com/netw-device-driver/ndd-grpc v0.1.1
	github.com/netw-device-driver/ndd-provider-srl v0.1.1
	github.com/netw-device-driver/ndd-provider-sros v0.1.0
	github.com/netw-device-driver/ndd-runtime v0.3.73
	github.com/netw-device-driver/netwdevpb v0.1.27
	github.com/openconfig/gnmi v0.0.0-20210707145734-c69a5df04b53
	github.com/pkg/errors v0.9.1
	github.com/r3labs/diff/v2 v2.13.6
	github.com/sirupsen/logrus v1.7.0
	github.com/spf13/cobra v1.2.1
	github.com/stoewer/go-strcase v1.2.0
	google.golang.org/grpc v1.39.0
	k8s.io/api v0.21.3
	k8s.io/apimachinery v0.21.3
	k8s.io/client-go v0.21.3
	sigs.k8s.io/controller-runtime v0.9.3
)
