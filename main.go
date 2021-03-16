package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/netw-device-driver/netw-device-driver-gnmi/ddriver"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	nddv1 "github.com/netw-device-driver/netw-device-controller/api/v1"
	corev1 "k8s.io/api/core/v1"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(nddv1.AddToScheme(scheme))
}

var (
	// Log generic logger
	natsServer string
	deviceName string
	debug      bool
)

// InitFlags function
func InitFlags(fs *pflag.FlagSet) {
	fs.StringVar(&natsServer, "nats-server", "",
		"The address the natsServer to subscribe to")

	fs.StringVar(&deviceName, "device-name", "leaf1",
		"Name of the device the driver serves")

	fs.BoolVar(&debug, "debug", false,
		"Debug control")
}

func main() {
	log.Info("setting up flags in netwdevicedriver...")

	InitFlags(pflag.CommandLine)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	if debug {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}

	// Get K8s client with scheme that includes the network device driver CRDs
	k8sclopts := client.Options{
		Scheme: scheme,
	}

	c, err := client.New(config.GetConfigOrDie(), k8sclopts)
	if err != nil {
		fmt.Println("failed to create client")
		os.Exit(1)
	}

	// get Network node information -> target infor, creds info, etc
	nnKey := types.NamespacedName{
		Namespace: "default",
		Name:      deviceName,
	}
	nn := &nddv1.NetworkNode{}
	if err := c.Get(context.TODO(), nnKey, nn); err != nil {
		log.WithError(err).Error("Failed to get NetworkNode")
		os.Exit(1)
	}

	log.Infof("Network Node info: %v", *nn)

	secretKey := nn.CredentialsKey()
	credsSecret := &corev1.Secret{}
	if err := c.Get(context.TODO(), secretKey, credsSecret); err != nil {
		log.WithError(err).Error("Failed to get Secret")
		os.Exit(1)
	}

	username := strings.TrimSuffix(string(credsSecret.Data["username"]), "\n")
	password := strings.TrimSuffix(string(credsSecret.Data["password"]), "\n")

	opts := []ddriver.Option{
		ddriver.WithServer(&natsServer),
		ddriver.WithDeviceName(&deviceName),
		ddriver.WithK8sClient(&c),
	}

	popts := []ddriver.GnmiProtocolOption{
		ddriver.WithTarget(&nn.Spec.Target.Address),
		ddriver.WithUsername(&username),
		ddriver.WithPassword(&password),
		ddriver.WithSkipVerify(nn.Spec.Target.SkipVerify),
		ddriver.WithEncoding(&nn.Spec.Target.Encoding),
	}
	d := ddriver.NewDeviceDriver(opts, popts...)

	for {
		if err := d.DiscoverDeviceDetails(); err != nil {
			// TODO update status in k8s API
			log.WithError(err).Error("Network Node discovery failed")
			time.Sleep(60 * time.Second)
		} else {
			break
		}
	}

	// TODO update status in k8s API

	d.InitDeviceDriverControllers()
}
