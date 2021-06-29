module github.com/netw-device-driver/netw-device-driver-gnmi

go 1.15

require (
	github.com/cloudflare/cfssl v1.5.0
	github.com/evanphx/json-patch v4.9.0+incompatible
	github.com/google/go-cmp v0.5.4
	github.com/karimra/gnmic v0.9.1
	github.com/mitchellh/mapstructure v1.3.3 // indirect
	github.com/netw-device-driver/netw-device-controller v0.1.6
	github.com/netw-device-driver/netwdevpb v0.1.23
	github.com/openconfig/gnmi v0.0.0-20210226144353-8eae1937bf84
	github.com/r3labs/diff/v2 v2.12.0
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/pflag v1.0.5
	github.com/stoewer/go-strcase v1.2.0
	golang.org/x/net v0.0.0-20201216054612-986b41b23924 // indirect
	google.golang.org/grpc v1.37.1
	gopkg.in/check.v1 v1.0.0-20200902074654-038fdea0a05b // indirect
	k8s.io/api v0.20.4
	k8s.io/apimachinery v0.20.4
	k8s.io/client-go v0.20.2
	sigs.k8s.io/controller-runtime v0.8.3
)
