/*
 * Copyright 2018-present Open Networking Foundation

 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at

 * http://www.apache.org/licenses/LICENSE-2.0

 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package core

import (
	"github.com/opencord/openolt-scale-tester/config"
	"github.com/opencord/voltha-lib-go/v2/pkg/log"
	oop "github.com/opencord/voltha-protos/v2/go/openolt"
	"strconv"
	"time"
)

func init() {
	_, _ = log.AddPackage(log.JSON, log.DebugLevel, nil)
}

type SubscriberKey struct {
	SubscriberName string
}

type OnuDevice struct {
	SerialNum                string                        `json:"onuSerialNum"`
	OnuID                    uint32                        `json:"onuID"`
	PonIntf                  uint32                        `json:"ponIntf"`
	OnuProvisionStartTime    time.Time                     `json:"onuProvisionStartTime"`
	OnuProvisionEndTime      time.Time                     `json:"onuProvisionEndTime"`
	OnuProvisionDurationInMs int64                         `json:"onuProvisionDurationInMilliSec"`
	Reason                   string                        `json:"reason"` // If provisioning failed, this specifies the reason.
	SubscriberMap            map[SubscriberKey]*Subscriber `json:"subscriberMap"`
	openOltClient            oop.OpenoltClient
	testConfig               *config.OpenOltScaleTesterConfig
	rsrMgr                   *OpenOltResourceMgr
}

func (onu *OnuDevice) Start(oltCh chan bool) {
	onu.SubscriberMap = make(map[SubscriberKey]*Subscriber)
	onuCh := make(chan bool)
	var subs uint

	log.Infow("onu-provision-started-from-onu-manager", log.Fields{"onuID": onu.OnuID, "ponIntf": onu.PonIntf})

	for subs = 0; subs < onu.testConfig.SubscribersPerOnu; subs++ {
		subsName := onu.SerialNum + "-" + strconv.Itoa(int(subs))
		subs := Subscriber{
			SubscriberName: subsName,
			OnuID:          onu.OnuID,
			UniID:          uint32(subs),
			PonIntf:        onu.PonIntf,
			UniPortNo:      MkUniPortNum(onu.PonIntf, onu.OnuID, uint32(subs)),
			Ctag:           GetCtag(onu.testConfig.WorkflowName, onu.PonIntf),
			Stag:           GetStag(onu.testConfig.WorkflowName, onu.PonIntf),
			OpenOltClient:  onu.openOltClient,
			TestConfig:     onu.testConfig,
			RsrMgr:         onu.rsrMgr,
		}
		subsKey := SubscriberKey{subsName}
		onu.SubscriberMap[subsKey] = &subs

		log.Infow("subscriber-provision-started-from-onu-manager", log.Fields{"subsName": subsName})
		// Start provisioning the subscriber
		go subs.Start(onuCh)

		// Wait for subscriber provision to complete
		<-onuCh

		log.Infow("subscriber-provision-completed-from-onu-manager", log.Fields{"subsName": subsName})

		//Sleep for configured interval before provisioning another subscriber
		time.Sleep(time.Duration(onu.testConfig.TimeIntervalBetweenSubs))
	}
	// Indicate that the ONU provisioning is complete
	oltCh <- true

	log.Infow("onu-provision-completed-from-onu-manager", log.Fields{"onuID": onu.OnuID, "ponIntf": onu.PonIntf})
}
