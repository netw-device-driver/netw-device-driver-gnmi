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

package srl

import (
	"context"

	"github.com/karimra/gnmic/collector"
	ndddvrv1 "github.com/netw-device-driver/ndd-core/apis/dvr/v1"
	nddv1 "github.com/netw-device-driver/ndd-runtime/apis/common/v1"
	"github.com/netw-device-driver/ndd-runtime/pkg/logging"
	"github.com/netw-device-driver/ndd-runtime/pkg/utils"
	"github.com/netw-device-driver/netw-device-driver-gnmi/internal/devices"
	"github.com/netw-device-driver/netw-device-driver-gnmi/internal/gnmic"
	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/pkg/errors"
)

const (
	state    = "STATE"
	config   = "CONFIG"
	encoding = "JSON"
	//errors
	errGnmiCreateGetRequest    = "gnmi create get request error"
	errGnmiGet                 = "gnmi get error "
	errGnmiHandleGetResponse   = "gnmi get response error"
	errGnmiCreateSetRequest    = "gnmi create set request error"
	errGnmiSet                 = "gnmi set error "
	errGnmiCreateDeleteRequest = "gnmi create delete request error"
)

func init() {
	devices.Register(nddv1.DeviceTypeSROS, func() devices.Device {
		return new(srl)
	})
}

type srl struct {
	target        *collector.Target
	log           logging.Logger
	deviceDetails *ndddvrv1.DeviceDetails
}

func (d *srl) Init(opts ...devices.DeviceOption) error {
	for _, o := range opts {
		o(d)
	}
	return nil
}

func (s *srl) WithTarget(*collector.Target) {}

func (s *srl) WithLogging(logging.Logger) {}

func (d *srl) Discover(ctx context.Context) (*ndddvrv1.DeviceDetails, error) {
	d.log.Debug("Discover SROS details ...")
	devDetails := &ndddvrv1.DeviceDetails{}

	// TODO

	return devDetails, nil
}

func (d *srl) GetConfig(ctx context.Context) (map[string]interface{}, error) {
	var err error
	var p string
	var req *gnmi.GetRequest
	var rsp *gnmi.GetResponse

	p = "/"
	req, err = gnmic.CreateGetRequest(&p, utils.StringPtr(config), utils.StringPtr(encoding))
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

func (d *srl) Get(ctx context.Context, p *string) (map[string]interface{}, error) {
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

func (d *srl) Update(ctx context.Context, p *string, data []byte) (*gnmi.SetResponse, error) {
	req, err := gnmic.CreateSetRequest(p, data)
	if err != nil {
		d.log.Debug(errGnmiCreateSetRequest, "error", err)
		return nil, errors.Wrap(err, errGnmiCreateSetRequest)
	}
	resp, err := d.target.Set(ctx, req)
	if err != nil {
		d.log.Debug(errGnmiSet, "error", err)
		return nil, errors.Wrap(err, errGnmiSet)
	}
	d.log.Debug("response:", "resp", resp)
	return resp, nil
}

func (d *srl) Delete(ctx context.Context, p *string) (*gnmi.SetResponse, error) {
	req, err := gnmic.CreateDeleteRequest(p)
	if err != nil {
		d.log.Debug(errGnmiCreateDeleteRequest, "error", err)
		return nil, errors.Wrap(err, errGnmiCreateDeleteRequest)
	}
	resp, err := d.target.Set(ctx, req)
	if err != nil {
		d.log.Debug(errGnmiSet, "error", err)
		return nil, errors.Wrap(err, errGnmiSet)
	}
	d.log.Debug("response:", "resp", resp)
	return resp, nil
}
