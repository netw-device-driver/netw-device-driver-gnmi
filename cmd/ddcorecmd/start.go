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
	"os"

	"github.com/netw-device-driver/ndd-runtime/pkg/logging"
	"github.com/netw-device-driver/netw-device-driver-gnmi/internal/dd"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	grpcServerAddress string
	deviceName        string
	namespace         string
)

const (
	// Errors
	errCreateDeviceDriver = "cannot create device driver"
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
		log.WithValues("deviceName", deviceName, "grpcServerAddress", grpcServerAddress)

		log.Debug("started gnmi device driver")

		opts := []dd.Option{
			dd.WithDeviceName(&deviceName),
			dd.WithNameSpace(&namespace),
			dd.WithScheme(scheme),
			dd.WithLogger(log.WithValues("deviceDriver", deviceName)),
			dd.WithGrpcServer(&grpcServerAddress),
			dd.WithDebug(&debug),
		}
		d, err := dd.NewDeviceDriver(config.GetConfigOrDie(), opts...)
		if err != nil {
			return errors.Wrap(err, errCreateDeviceDriver)
		}

		if err := d.Run(); err != nil {
			return errors.Wrap(err, "problem running device driver")
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
	startCmd.Flags().StringVarP(&grpcServerAddress, "grpc-server-address", "s", os.Getenv("POD_IP"), "The address of the grpc server binds to.")
	startCmd.Flags().StringVarP(&deviceName, "device-name", "n", "", "Name of the device the device driver serves")
	startCmd.Flags().StringVarP(&namespace, "namespace", "", "", "Namespace the configuration is deployed on")
}
