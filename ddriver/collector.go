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
	"sync"
	"time"

	"github.com/karimra/gnmic/collector"
	"github.com/netw-device-driver/netw-device-driver-gnmi/pkg/gnmic"
	log "github.com/sirupsen/logrus"
)

const (
	defaultTargetReceivebuffer = 1000
	defaultLockRetry           = 5 * time.Second
	defaultRetryTimer          = 10 * time.Second
)

type Collector struct {
	TargetReceiveBuffer uint
	RetryTimer          time.Duration
	Target              *collector.Target
	targetSubRespChan   chan *collector.SubscribeResponse
	targetSubErrChan    chan *collector.TargetError
	Subscriptions       map[string]*Subscription
	SubscriptionsMutex  sync.RWMutex
}

type Subscription struct {
	StopCh   chan bool
	CancelFn context.CancelFunc
}

func NewCollector(t *collector.Target) *Collector {
	return &Collector{
		Target:              t,
		Subscriptions:       make(map[string]*Subscription),
		SubscriptionsMutex:  sync.RWMutex{},
		TargetReceiveBuffer: defaultTargetReceivebuffer,
		RetryTimer:          defaultRetryTimer,
	}
}

func (c *Collector) StopSubscription(sub *string) {
	log.Infof("Stop subscription %s", *sub)
	c.Subscriptions[*sub].StopCh <- true // trigger quit

	log.Infof("subscription %s stopped ...", *sub)
}

func (c *Collector) StartSubscription(dctx context.Context, subName *string, sub *[]string) {
	log.Infof("Start subscription %s", *sub)
	// initialize new subscription
	ctx, cancel := context.WithCancel(dctx)

	c.Subscriptions[*subName] = &Subscription{
		StopCh:   make(chan bool),
		CancelFn: cancel,
	}
	//subName := gnmic.SubName(subname)

	req, err := gnmic.CreateSubscriptionRequest(sub)
	if err != nil {
		log.WithError(err).Error("Create subscription request failed")
	}

	go func() {
		c.Target.Subscribe(ctx, req, *subName)
	}()
	log.Infof("subscription %s started ...", *sub)

	for {
		select {
		case <-c.Subscriptions[*subName].StopCh: // execute quit
			c.Subscriptions[*subName].CancelFn()
			c.SubscriptionsMutex.Lock()
			delete(c.Subscriptions, *subName)
			c.SubscriptionsMutex.Unlock()
			log.Infof("Cancel/Deleted subscription %s", *sub)
			return
		}

	}

}
