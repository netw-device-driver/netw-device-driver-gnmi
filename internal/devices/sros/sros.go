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

package sros

/*
import (
	"bytes"
	"context"

	"github.com/karimra/gnmic/collector"
	ndrv1 "github.com/netw-device-driver/ndd-core/apis/dvr/v1"
	config "github.com/netw-device-driver/ndd-grpc/config/configpb"
	srosv1 "github.com/netw-device-driver/ndd-provider-sros/apis/sros/v1"
	"github.com/netw-device-driver/ndd-runtime/pkg/logging"
	"github.com/netw-device-driver/ndd-runtime/pkg/utils"
	"github.com/netw-device-driver/netw-device-driver-gnmi/internal/devices"
	"github.com/netw-device-driver/netw-device-driver-gnmi/internal/gnmic"
	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/pkg/errors"
	"github.com/yndd/ndd-yang/pkg/parser"
)

const (
	State         = "STATE"
	Configuration = "CONFIG"
	encoding      = "JSON"
	//errors
	errGnmiCreateGetRequest    = "gnmi create get request error"
	errGnmiGet                 = "gnmi get error "
	errGnmiHandleGetResponse   = "gnmi get response error"
	errGnmiCreateSetRequest    = "gnmi create set request error"
	errGnmiSet                 = "gnmi set error "
	errGnmiCreateDeleteRequest = "gnmi create delete request error"
)

func init() {
	devices.Register(srosv1.DeviceTypeSROS, func() devices.Device {
		return new(sros)
	})
}

type sros struct {
	target        *collector.Target
	log           logging.Logger
	parser        *parser.Parser
	deviceDetails *ndrv1.DeviceDetails
}

func (d *sros) Init(opts ...devices.DeviceOption) error {
	for _, o := range opts {
		o(d)
	}
	return nil
}

func (d *sros) WithTarget(target *collector.Target) {
	d.target = target
}

func (d *sros) WithLogging(log logging.Logger) {
	d.log = log
}

func (d *sros) WithParser(log logging.Logger) {
	d.parser = parser.NewParser(parser.WithLogger((log)))
}

func (d *sros) Discover(ctx context.Context) (*ndrv1.DeviceDetails, error) {
	d.log.Debug("Discover SROS details ...")
	devDetails := &ndrv1.DeviceDetails{}

	// TODO

	return devDetails, nil
}

func (d *sros) GetConfig(ctx context.Context) (map[string]interface{}, error) {
	var err error
	var p string
	var req *gnmi.GetRequest
	var rsp *gnmi.GetResponse

	p = "/"
	req, err = gnmic.CreateGetRequest(&p, utils.StringPtr(Configuration), utils.StringPtr(encoding))
	if err != nil {
		return nil, errors.Wrap(err, errGnmiCreateGetRequest)
	}
	rsp, err = d.target.Get(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, errGnmiGet)
	}

	u, err := gnmic.HandleGetResponse(rsp)
	if err != nil {
		return nil, errors.Wrap(err, errGnmiHandleGetResponse)
	}

	for _, update := range u {
		d.log.Debug("GetConfig", "response", update)
		return update.Values, nil
	}
	return nil, nil
}

func (d *sros) Get(ctx context.Context, p *string) (map[string]interface{}, error) {
	var err error
	var req *gnmi.GetRequest
	var rsp *gnmi.GetResponse

	req, err = gnmic.CreateGetRequest(p, utils.StringPtr("CONFIG"), utils.StringPtr("JSON_IETF"))
	if err != nil {
		d.log.Debug(errGnmiCreateGetRequest, "error", err)
		return nil, errors.Wrap(err, errGnmiCreateGetRequest)
	}
	rsp, err = d.target.Get(ctx, req)
	if err != nil {
		d.log.Debug(errGnmiGet, "error", err)
		return nil, errors.Wrap(err, errGnmiGet)
	}
	u, err := gnmic.HandleGetResponse(rsp)
	if err != nil {
		d.log.Debug(errGnmiHandleGetResponse, "error", err)
		return nil, errors.Wrap(err, errGnmiHandleGetResponse)
	}
	for _, update := range u {
		d.log.Debug("GetConfig", "response", update)
		return update.Values, nil
	}
	return nil, nil
}

func (d *sros) Update(ctx context.Context, u []*config.Update) (*gnmi.SetResponse, error) {
	gnmiPrefix, err := collector.CreatePrefix("", "")
	if err != nil {
		d.log.Debug(errGnmiSet, "error", err)
		return nil, errors.Wrap(err, "prefix parse error")
	}

	updates := make([]*gnmi.Update, 0)
	for _, upd := range u {
		updates = append(updates, &gnmi.Update{
			Path: d.parser.ConfigPath2GnmiPath(upd.Path),
			Val: &gnmi.TypedValue{
				Value: &gnmi.TypedValue_JsonIetfVal{
					JsonIetfVal: bytes.Trim(upd.Value, " \r\n\t"),
				},
			},
		})
	}

	req := &gnmi.SetRequest{
		Prefix: gnmiPrefix,
		Update: updates,
	}

	resp, err := d.target.Set(ctx, req)
	if err != nil {
		d.log.Debug(errGnmiSet, "error", err)
		return nil, errors.Wrap(err, errGnmiSet)
	}
	d.log.Debug("response:", "resp", resp)
	return resp, nil
}

func (d *sros) Delete(ctx context.Context, p []*config.Path) (*gnmi.SetResponse, error) {
	gnmiPrefix, err := collector.CreatePrefix("", "")
	if err != nil {
		d.log.Debug(errGnmiSet, "error", err)
		return nil, errors.Wrap(err, "prefix parse error")
	}

	deletes := make([]*gnmi.Path, 0)
	for _, del := range p {
		dp := d.parser.ConfigPath2GnmiPath(del)
		deletes = append(deletes, dp)
	}

	req := &gnmi.SetRequest{
		Prefix: gnmiPrefix,
		Delete: deletes,
	}

	resp, err := d.target.Set(ctx, req)
	if err != nil {
		d.log.Debug(errGnmiSet, "error", err)
		return nil, errors.Wrap(err, errGnmiSet)
	}
	d.log.Debug("response:", "resp", resp)

	return resp, nil
}

func (d *sros) Set(ctx context.Context, u []*config.Update, p []*config.Path) (*gnmi.SetResponse, error) {
	gnmiPrefix, err := collector.CreatePrefix("", "")
	if err != nil {
		d.log.Debug(errGnmiSet, "error", err)
		return nil, errors.Wrap(err, "prefix parse error")
	}

	deletes := make([]*gnmi.Path, 0)
	for _, del := range p {
		dp := d.parser.ConfigPath2GnmiPath(del)
		deletes = append(deletes, dp)
	}

	updates := make([]*gnmi.Update, 0)
	for _, upd := range u {
		updates = append(updates, &gnmi.Update{
			Path: d.parser.ConfigPath2GnmiPath(upd.Path),
			Val: &gnmi.TypedValue{
				Value: &gnmi.TypedValue_JsonIetfVal{
					JsonIetfVal: bytes.Trim(upd.Value, " \r\n\t"),
				},
			},
		})
	}

	req := &gnmi.SetRequest{
		Prefix: gnmiPrefix,
		Update: updates,
		Delete: deletes,
	}

	resp, err := d.target.Set(ctx, req)
	if err != nil {
		d.log.Debug(errGnmiSet, "error", err)
		return nil, errors.Wrap(err, errGnmiSet)
	}
	d.log.Debug("response:", "resp", resp)
	return resp, nil
}
*/
