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
	"fmt"
	"github.com/opencord/voltha-protos/v5/go/tech_profile"
	"sync"

	"github.com/opencord/openolt-scale-tester/config"
	"github.com/opencord/voltha-lib-go/v7/pkg/log"
	oop "github.com/opencord/voltha-protos/v5/go/openolt"
	"golang.org/x/net/context"
)

const (
	SUBSCRIBER_PROVISION_SUCCESS = iota
	TP_INSTANCE_CREATION_FAILED
	FLOW_ID_GENERATION_FAILED
	FLOW_ADD_FAILED
	SCHED_CREATION_FAILED
	QUEUE_CREATION_FAILED
)

const (
	UniPortName = "olt-{1}/pon-{%d}/onu-{%d}/uni-{%d}"
)

var Reason = [...]string{
	"SUBSCRIBER_PROVISION_SUCCESS",
	"TP_INSTANCE_CREATION_FAILED",
	"FLOW_ID_GENERATION_FAILED",
	"FLOW_ADD_FAILED",
	"SCHED_CREATION_FAILED",
	"QUEUE_CREATION_FAILED",
}

func ReasonCodeToReasonString(reasonCode int) string {
	return Reason[reasonCode]
}

type Subscriber struct {
	SubscriberName string   `json:"subscriberName"`
	OnuID          uint32   `json:"onuID"`
	UniID          uint32   `json:"uniID"`
	PonIntf        uint32   `json:"ponIntf"`
	UniPortNo      uint32   `json:"uniPortNo"`
	Ctag           uint32   `json:"ctag"`
	Stag           uint32   `json:"stag"`
	GemPortIDs     []uint32 `json:"gemPortIds"`
	AllocIDs       []uint32 `json:"allocIds"`
	FlowIDs        []uint32 `json:"flowIds"`
	Reason         string   `json:"reason"`

	FailedFlowCnt  uint32 `json:"failedFlowCnt"`
	SuccessFlowCnt uint32 `json:"successFlowCnt"`

	FailedSchedCnt  uint32 `json:"failedSchedCnt"`
	SuccessSchedCnt uint32 `json:"successShedCnt"`

	FailedQueueCnt  uint32 `json:"failedQueueCnt"`
	SuccessQueueCnt uint32 `json:"successQueueCnt"`

	FailedFlows  []oop.Flow             `json:"failedFlows"`
	FailedScheds []oop.TrafficScheduler `json:"failedScheds"`
	FailedQueues []oop.TrafficQueue     `json:"failedQueues"`

	TpInstance    map[int]*tech_profile.TechProfileInstance
	OpenOltClient oop.OpenoltClient
	TestConfig    *config.OpenOltScaleTesterConfig
	RsrMgr        *OpenOltResourceMgr
	subWg         *sync.WaitGroup
}

func (subs *Subscriber) Start(isGroup bool) {

	logger.Infow(nil, "workflow-deploy-started-for-subscriber", log.Fields{"subsName": subs.SubscriberName})

	subs.TpInstance = make(map[int]*tech_profile.TechProfileInstance)

	for _, tpID := range subs.TestConfig.TpIDList {
		uniPortName := fmt.Sprintf(UniPortName, subs.PonIntf, subs.OnuID, subs.UniID)
		tpInstInterface, err := subs.RsrMgr.TechprofileRef.CreateTechProfileInstance(context.Background(), uint32(tpID), uniPortName, subs.PonIntf)
		// TODO: Assumes the techprofile is of type TechProfile (XGPON, GPON). But it could also be EPON TechProfile type. But we do not support that at the moment, so it is OK.
		subs.TpInstance[tpID] = tpInstInterface.(*tech_profile.TechProfileInstance)

		if err != nil {
			logger.Errorw(nil, "error-creating-tp-instance-for-subs",
				log.Fields{"subsName": subs.SubscriberName, "onuID": subs.OnuID, "tpID": tpID})

			subs.Reason = ReasonCodeToReasonString(TP_INSTANCE_CREATION_FAILED)
			return
		}
	}

	go DeployWorkflow(subs, isGroup)

	logger.Infow(nil, "workflow-deploy-started-for-subscriber", log.Fields{"subsName": subs.SubscriberName})
}
