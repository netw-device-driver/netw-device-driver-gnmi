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

package ddriver

import (
	"context"

	"github.com/netw-device-driver/netw-device-driver-gnmi/pkg/grpcc"
	"github.com/netw-device-driver/netwdevpb"
)

func deviationUpdate(ctx context.Context, target, resource *string) (*netwdevpb.DeviationUpdateReply, error) {
	c := &grpcc.Client{
		Insecure:   true,
		SkipVerify: true,
		Target:     *target,
	}
	req := &netwdevpb.DeviationUpdate{
		Resource: *resource,
	}
	return c.DeviationUpdate(ctx, req)
}
