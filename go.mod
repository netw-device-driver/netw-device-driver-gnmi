module github.com/netw-device-driver/netw-device-driver-gnmi

go 1.16

require (
	github.com/evanphx/json-patch v4.11.0+incompatible
	github.com/karimra/gnmic v0.18.0
	github.com/netw-device-driver/ndd-core v0.1.10
	github.com/netw-device-driver/ndd-grpc v0.1.33
	github.com/netw-device-driver/ndd-runtime v0.4.117
	github.com/openconfig/gnmi v0.0.0-20210707145734-c69a5df04b53
	github.com/pkg/errors v0.9.1
	github.com/spf13/cobra v1.2.1
	github.com/stoewer/go-strcase v1.2.0
	github.com/wI2L/jsondiff v0.1.0
	github.com/yndd/ndd-provider-srl v0.1.1
	github.com/yndd/ndd-yang v0.1.87
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20210630005230-0f9fa26af87c // indirect
	google.golang.org/grpc v1.39.0
	k8s.io/api v0.21.3
	k8s.io/apiextensions-apiserver v0.21.3 // indirect
	k8s.io/apimachinery v0.21.3
	k8s.io/client-go v0.21.3
	sigs.k8s.io/controller-runtime v0.9.3
)
