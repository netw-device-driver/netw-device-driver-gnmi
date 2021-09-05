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

package dd

import (
	"context"
	"sync"
	"time"

	"github.com/karimra/gnmic/target"
	"github.com/netw-device-driver/ndd-runtime/pkg/logging"
	"github.com/netw-device-driver/netw-device-driver-gnmi/internal/gnmic"
	"github.com/pkg/errors"
)

const (
	defaultTargetReceivebuffer = 1000
	defaultLockRetry           = 5 * time.Second
	defaultRetryTimer          = 10 * time.Second

	// errors
	errCreateSubscriptionRequest = "cannot create subscription request"
)

type Collector interface {
	Lock()
	Unlock()
	GetSubscription(subName string) bool
	StopSubscription(ctx context.Context, sub string) error
	StartSubscription(ctx context.Context, subName string, sub []string) error
}

// DeviceCollectorOption can be used to manipulate Options.
type DeviceCollectorOption func(*DeviceCollector)

// WithTargetLogger specifies how the object should log messages.
func WithDeviceCollectorLogger(log logging.Logger) DeviceCollectorOption {
	return func(o *DeviceCollector) {
		o.log = log
	}
}

type DeviceCollector struct {
	TargetReceiveBuffer uint
	RetryTimer          time.Duration
	Target              *target.Target
	//targetSubRespChan   chan *collector.SubscribeResponse
	//targetSubErrChan    chan *collector.TargetError
	Subscriptions map[string]*Subscription
	Mutex         sync.RWMutex
	log           logging.Logger
}

type Subscription struct {
	StopCh   chan bool
	CancelFn context.CancelFunc
}

func NewDeviceCollector(t *target.Target, opts ...DeviceCollectorOption) *DeviceCollector {
	c := &DeviceCollector{
		Target:              t,
		Subscriptions:       make(map[string]*Subscription),
		Mutex:               sync.RWMutex{},
		TargetReceiveBuffer: defaultTargetReceivebuffer,
		RetryTimer:          defaultRetryTimer,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *DeviceCollector) Lock() {
	c.Mutex.RLock()
}

func (c *DeviceCollector) Unlock() {
	c.Mutex.RUnlock()
}

func (c *DeviceCollector) GetSubscription(subName string) bool {
	if _, ok := c.Subscriptions[subName]; !ok {
		return true
	}
	return false
}

func (c *DeviceCollector) StopSubscription(ctx context.Context, sub string) error {
	c.log.WithValues("subscription", sub)
	c.log.Debug("subscription stop...")
	c.Subscriptions[sub].StopCh <- true // trigger quit

	c.log.Debug("subscription stopped")
	return nil
}

func (c *DeviceCollector) StartSubscription(dctx context.Context, subName string, sub []string) error {
	c.log.WithValues("subscription", sub)
	c.log.Debug("subscription start...")
	// initialize new subscription
	ctx, cancel := context.WithCancel(dctx)

	c.Subscriptions[subName] = &Subscription{
		StopCh:   make(chan bool),
		CancelFn: cancel,
	}

	req, err := gnmic.CreateSubscriptionRequest(&sub)
	if err != nil {
		c.log.Debug(errCreateSubscriptionRequest, "error", err)
		return errors.Wrap(err, errCreateSubscriptionRequest)
	}

	go func() {
		c.Target.Subscribe(ctx, req, subName)
	}()
	c.log.Debug("subscription started ...")

	for {
		select {
		case <-c.Subscriptions[subName].StopCh: // execute quit
			c.Subscriptions[subName].CancelFn()
			c.Mutex.Lock()
			delete(c.Subscriptions, subName)
			c.Mutex.Unlock()
			c.log.Debug("subscription cancelled")
			return nil
		}
	}
}
