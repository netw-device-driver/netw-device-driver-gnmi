module github.com/netw-device-driver/netw-device-driver-gnmi

go 1.15

require (
	github.com/cloudflare/cfssl v1.5.0
	github.com/evanphx/json-patch v4.9.0+incompatible
	github.com/google/gnxi v0.0.0-20210301094713-a533bddd461b
	github.com/netw-device-driver/netw-device-controller v0.1.3
	github.com/netw-device-driver/netwdevpb v0.1.15
	github.com/openconfig/gnmi v0.0.0-20210226144353-8eae1937bf84
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/pflag v1.0.5
	google.golang.org/grpc v1.36.0
	k8s.io/api v0.20.4
	k8s.io/apimachinery v0.20.4
	k8s.io/client-go v0.20.2
	sigs.k8s.io/controller-runtime v0.8.3
)
