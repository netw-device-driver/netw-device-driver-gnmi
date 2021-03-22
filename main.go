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
	//natsServer         string
	cacheServerAddress string
	deviceName         string
	debug              bool
	rootCaCsrTemplate  = "/templates/ca/csr-root-ca.json"
	certCsrTemplate    = "/templates/ca/csr.json"
)

// InitFlags function
func InitFlags(fs *pflag.FlagSet) {
	/*
		fs.StringVar(&natsServer, "nats-server", "",
			"The address forthe natsServer to subscribe to")
	*/

	fs.StringVar(&cacheServerAddress, "cache-server-address", "",
		"The address of the cache server")

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

	/*
		if debug {
			log.SetLevel(log.DebugLevel)
			cfssllog.Level = cfssllog.LevelDebug
		} else {
			log.SetLevel(log.InfoLevel)
			cfssllog.Level = cfssllog.LevelError
		}

		tpl, err := template.ParseFiles(rootCaCsrTemplate)
		if err != nil {
			log.WithError(err).Error("failed to parse rootCACsrTemplate")
		}
		_, err = util.GenerateRootCa(tpl, util.CaRootInput{Prefix: deviceName})
		if err != nil {
			log.WithError(err).Error("failed to generate rootCa")
		}

		// generate proxy CA
		certTpl, err := template.ParseFiles(certCsrTemplate)
		if err != nil {
			log.WithError(err).Error("failed to parse certCsrTemplate")
		}

		_, err = util.GenerateCert(
			deviceName,
			path.Join(util.DirLabCAroot, "root-ca.pem"),
			path.Join(util.DirLabCAroot, "root-ca-key.pem"),
			certTpl,
		)
	*/

	// get cacheServerAddress ip from the environment
	cacheServerAddress = os.Getenv("POD_IP") + ":" + strings.Split(cacheServerAddress, ":")[1]
	log.Infof("proxy cacheServerAddress: %s", cacheServerAddress)

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
		//ddriver.WithNatsServer(&natsServer),
		ddriver.WithCacheServer(&cacheServerAddress),
		ddriver.WithDeviceName(&deviceName),
		ddriver.WithK8sClient(&c),
	}

	popts := []ddriver.GnmiProtocolOption{
		ddriver.WithTarget(nn.Spec.Target.Address),
		ddriver.WithUsername(&username),
		ddriver.WithPassword(&password),
		ddriver.WithSkipVerify(*nn.Spec.Target.SkipVerify),
		ddriver.WithEncoding(nn.Spec.Target.Encoding),
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

	d.InitDeviceDriverControllers()
}
