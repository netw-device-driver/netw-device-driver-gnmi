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

package ddcorecmd

import (
	"context"
	"os"
	"strings"
	"time"

	ndddvrv1 "github.com/netw-device-driver/ndd-core/apis/dvr/v1"
	"github.com/netw-device-driver/ndd-runtime/pkg/logging"
	"github.com/netw-device-driver/ndd-runtime/pkg/resource"
	"github.com/netw-device-driver/ndd-runtime/pkg/utils"
	"github.com/netw-device-driver/netw-device-driver-gnmi/internal/ddriver"
	"github.com/netw-device-driver/netw-device-driver-gnmi/internal/devices"
	"github.com/netw-device-driver/netw-device-driver-gnmi/internal/jsonutils"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	cacheServerAddress string
	deviceName         string
	autoPilot          bool
	namespace          string
)

const (
	// Errors
	errCreateClient          = "cannot create k8s client"
	errGetNetworkNode        = "cannot get NetworkNode"
	errGetSecret             = "cannot get Secret"
	errCreateDeviceDriver    = "cannot create device driver"
	errUpdateDeviceDriver    = "cannot update device driver"
	errDeviceNotSupported    = "unsupported device kind"
	errDeviceInitFailed      = "cannot initialize the device"
	errDeviceDiscoveryFailed = "cannot discover device"
	errDeviceGetConfigFailed = "cannot get device config"
	errUpdateConfigmapFailed = "cannot update config map"
)

// startCmd represents the start command for the gnmic device driver
var startCmd = &cobra.Command{
	Use:          "start",
	Short:        "start gnmi device driver",
	Long:         "start gnmi device driver",
	Aliases:      []string{"start"},
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		zlog := zap.New(zap.UseDevMode(debug), zap.JSONEncoder())
		log := logging.NewLogrLogger(zlog.WithName("ddgnmi"))
		log.WithValues("deviceName", deviceName, "cacheServerAddress", cacheServerAddress, "autoPilot", autoPilot)
		log.Debug("started gnmi device driver")

		// Get K8s client with scheme that includes the network device driver CRDs
		k8sclopts := client.Options{
			Scheme: scheme,
		}
		c, err := client.New(config.GetConfigOrDie(), k8sclopts)
		if err != nil {
			return errors.Wrap(err, errCreateClient)
		}

		// get Network node information -> target info, creds info, etc
		nnKey := types.NamespacedName{
			Namespace: namespace,
			Name:      deviceName,
		}
		nn := &ndddvrv1.NetworkNode{}
		if err := c.Get(context.TODO(), nnKey, nn); err != nil {
			return errors.Wrap(err, errGetNetworkNode)
		}
		log.Debug("Network Node", "info", nn)

		secretKey := types.NamespacedName{
			Namespace: namespace,
			Name:      nn.GetTargetCredentialsName(),
		}
		credsSecret := &corev1.Secret{}
		if err := c.Get(context.TODO(), secretKey, credsSecret); err != nil {
			return errors.Wrap(err, errGetSecret)
		}

		username := strings.TrimSuffix(string(credsSecret.Data["username"]), "\n")
		password := strings.TrimSuffix(string(credsSecret.Data["password"]), "\n")

		log.Debug("User info", "username", username, "password", password)

		// initialize device driver
		opts := []ddriver.Option{
			//ddriver.WithNatsServer(&natsServer),
			ddriver.WithContext(),
			ddriver.WithLogger(log.WithValues("deviceDriver", deviceName)),
			ddriver.WithEstablisher(ddriver.NewAPIEstablisher(resource.ClientApplicator{
				Client:     c,
				Applicator: resource.NewAPIPatchingApplicator(c),
			}, log, deviceName, namespace, os.Getenv("POD_NAMESPACE"))),
			ddriver.WithCacheServer(&cacheServerAddress),
			ddriver.WithDeviceName(&deviceName),
			ddriver.WithK8sClient(&c),
			ddriver.WithTargetName(&deviceName),
			ddriver.WithTargetAddress(nn.Spec.Target.Address),
			ddriver.WithUsername(&username),
			ddriver.WithPassword(&password),
			ddriver.WithSkipVerify(nn.Spec.Target.SkipVerify),
			ddriver.WithInsecure(utils.BoolPtr(false)),
			ddriver.WithTLSCA(utils.StringPtr("")),
			ddriver.WithTLSCert(utils.StringPtr("")),
			ddriver.WithTLSKey(utils.StringPtr("")),
			ddriver.WithGzip(utils.BoolPtr(false)),
			ddriver.WithTimeout(10 * time.Second),
			//ddriver.WithEncoding(nn.Spec.Target.Encoding),
			ddriver.WithAutoPilot(&autoPilot),
			ddriver.WithDebug(&debug),
		}

		d, err := ddriver.NewDeviceDriver(opts...)
		if err != nil {
			return errors.Wrap(err, errCreateDeviceDriver)
		}

		// discover the device and retry until we can successfully discover the device
		for {
			deviceKind, err := d.Discoverer.Discover(d.Ctx)
			if err != nil {
				// update status with nil information
				d.DeviceDetails = &ndddvrv1.DeviceDetails{
					HostName:     &deviceName,
					Kind:         new(string),
					SwVersion:    new(string),
					MacAddress:   new(string),
					SerialNumber: new(string),
				}
				if err := d.Objects.UpdateDeviceStatusNotReady(d.Ctx, d.DeviceDetails); err != nil {
					return errors.Wrap(err, errUpdateDeviceDriver)
				}
				log.Debug("Network Node discovery failed")
				// retry in 60 sec
				time.Sleep(60 * time.Second)
			} else {
				// initializer
				log.Debug("device kind", "kind", deviceKind)
				deviceInitializer, ok := devices.Devices[deviceKind]
				if !ok {
					log.Debug(errDeviceNotSupported, "error", err)
					return errors.Wrap(err, errDeviceNotSupported)
				}
				d.Device = deviceInitializer()
				// init the device
				if err := d.Device.Init(
					devices.WithLogging(log),
					devices.WithTarget(d.Target),
				); err != nil {
					log.Debug(errDeviceInitFailed, "error", err)
					return errors.Wrap(err, errDeviceInitFailed)
				}
				// get device details
				d.DeviceDetails, err = d.Device.Discover(d.Ctx)
				if err != nil {
					log.Debug(errDeviceDiscoveryFailed, "error", err)
					return errors.Wrap(err, errDeviceDiscoveryFailed)
				}
				log.Debug("DeviceDetails", "info", d.DeviceDetails)
				// get initial config
				d.InitialConfig, err = d.Device.GetConfig(d.Ctx)
				if err != nil {
					log.Debug(errDeviceGetConfigFailed, "error", err)
					return errors.Wrap(err, errDeviceGetConfigFailed)
				}

				// clean config and provide a string ptr to map in the configmap
				var cfgStringptr *string
				d.InitialConfig, cfgStringptr, err = jsonutils.CleanConfig2String(d.InitialConfig)

				// update configmap with the initial config
				if err := d.Objects.UpdateConfigMap(d.Ctx, cfgStringptr); err != nil {
					log.Debug(errUpdateConfigmapFailed, "error", err)
					return errors.Wrap(err, errUpdateConfigmapFailed)
				}
				// bring the device in ready status
				if err := d.Objects.UpdateDeviceStatusReady(d.Ctx, d.DeviceDetails); err != nil {
					log.Debug(errUpdateDeviceDriver, "error", err)
					return errors.Wrap(err, errUpdateDeviceDriver)
				}
				break
			}
		}
		if err := d.InitGrpcServer(); err != nil {
			return err
		}

		// wait until we get a signal that we received the subscriptions
		<-d.SubCh
		log.Debug("subscription info received")

		for {
		}

		//return nil
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
	startCmd.Flags().StringVarP(&cacheServerAddress, "cache-server-address", "s", os.Getenv("POD_IP"), "The address of the cache server binds to.")
	startCmd.Flags().StringVarP(&deviceName, "device-name", "n", "", "Name of the device the device driver serves")
	startCmd.Flags().BoolVarP(&autoPilot, "auto-pilot", "a", true,
		"Apply delta/diff changes to the config automatically when set to true, if set to false the device driver will report the delta and the operator should intervene what to do with the delta/diffs")
	startCmd.Flags().StringVarP(&namespace, "namespace", "", "", "Namespace the configuration is deployed on")
}
