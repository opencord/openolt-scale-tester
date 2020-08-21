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
	"errors"
	"strings"

	"github.com/opencord/openolt-scale-tester/config"
	"github.com/opencord/voltha-lib-go/v3/pkg/log"
	oop "github.com/opencord/voltha-protos/v3/go/openolt"
	tp_pb "github.com/opencord/voltha-protos/v3/go/tech_profile"
	"golang.org/x/net/context"
)

func init() {
	_, _ = log.AddPackage(log.JSON, log.DebugLevel, nil)
}

// A dummy struct to comply with the WorkFlow interface.
type DtWorkFlow struct {
}

func ProvisionDtNniTrapFlow(oo oop.OpenoltClient, config *config.OpenOltScaleTesterConfig, rsrMgr *OpenOltResourceMgr) error {
	_ = AddLldpFlow(oo, config, rsrMgr)

	return nil
}

func (dt DtWorkFlow) ProvisionScheds(subs *Subscriber) error {
	var trafficSched []*tp_pb.TrafficScheduler

	log.Info("provisioning-scheds")

	if trafficSched = getTrafficSched(subs, tp_pb.Direction_DOWNSTREAM); trafficSched == nil {
		log.Error("ds-traffic-sched-is-nil")
		return errors.New(ReasonCodeToReasonString(SCHED_CREATION_FAILED))
	}

	log.Debugw("Sending Traffic scheduler create to device",
		log.Fields{"Direction": tp_pb.Direction_DOWNSTREAM, "TrafficScheds": trafficSched})
	if _, err := subs.OpenOltClient.CreateTrafficSchedulers(context.Background(), &tp_pb.TrafficSchedulers{
		IntfId: subs.PonIntf, OnuId: subs.OnuID,
		UniId: subs.UniID, PortNo: subs.UniPortNo,
		TrafficScheds: trafficSched}); err != nil {
		log.Errorw("Failed to create traffic schedulers", log.Fields{"error": err})
		return errors.New(ReasonCodeToReasonString(SCHED_CREATION_FAILED))
	}

	if trafficSched = getTrafficSched(subs, tp_pb.Direction_UPSTREAM); trafficSched == nil {
		log.Error("us-traffic-sched-is-nil")
		return errors.New(ReasonCodeToReasonString(SCHED_CREATION_FAILED))
	}

	log.Debugw("Sending Traffic scheduler create to device",
		log.Fields{"Direction": tp_pb.Direction_UPSTREAM, "TrafficScheds": trafficSched})
	if _, err := subs.OpenOltClient.CreateTrafficSchedulers(context.Background(), &tp_pb.TrafficSchedulers{
		IntfId: subs.PonIntf, OnuId: subs.OnuID,
		UniId: subs.UniID, PortNo: subs.UniPortNo,
		TrafficScheds: trafficSched}); err != nil {
		log.Errorw("Failed to create traffic schedulers", log.Fields{"error": err})
		return errors.New(ReasonCodeToReasonString(SCHED_CREATION_FAILED))
	}

	return nil
}

func (dt DtWorkFlow) ProvisionQueues(subs *Subscriber) error {
	log.Info("provisioning-queues")

	var trafficQueues []*tp_pb.TrafficQueue
	if trafficQueues = getTrafficQueues(subs, tp_pb.Direction_DOWNSTREAM); trafficQueues == nil {
		log.Error("Failed to create traffic queues")
		return errors.New(ReasonCodeToReasonString(QUEUE_CREATION_FAILED))
	}

	// On receiving the CreateTrafficQueues request, the driver should create corresponding
	// downstream queues.
	log.Debugw("Sending Traffic Queues create to device",
		log.Fields{"Direction": tp_pb.Direction_DOWNSTREAM, "TrafficQueues": trafficQueues})
	if _, err := subs.OpenOltClient.CreateTrafficQueues(context.Background(),
		&tp_pb.TrafficQueues{IntfId: subs.PonIntf, OnuId: subs.OnuID,
			UniId: subs.UniID, PortNo: subs.UniPortNo,
			TrafficQueues: trafficQueues}); err != nil {
		log.Errorw("Failed to create traffic queues in device", log.Fields{"error": err})
		return errors.New(ReasonCodeToReasonString(QUEUE_CREATION_FAILED))
	}

	if trafficQueues = getTrafficQueues(subs, tp_pb.Direction_UPSTREAM); trafficQueues == nil {
		log.Error("Failed to create traffic queues")
		return errors.New(ReasonCodeToReasonString(QUEUE_CREATION_FAILED))
	}

	// On receiving the CreateTrafficQueues request, the driver should create corresponding
	// upstream queues.
	log.Debugw("Sending Traffic Queues create to device",
		log.Fields{"Direction": tp_pb.Direction_UPSTREAM, "TrafficQueues": trafficQueues})
	if _, err := subs.OpenOltClient.CreateTrafficQueues(context.Background(),
		&tp_pb.TrafficQueues{IntfId: subs.PonIntf, OnuId: subs.OnuID,
			UniId: subs.UniID, PortNo: subs.UniPortNo,
			TrafficQueues: trafficQueues}); err != nil {
		log.Errorw("Failed to create traffic queues in device", log.Fields{"error": err})
		return errors.New(ReasonCodeToReasonString(QUEUE_CREATION_FAILED))
	}

	return nil
}

func (dt DtWorkFlow) ProvisionEapFlow(subs *Subscriber) error {
	log.Info("dt-workflow-does-not-require-eap-support--nothing-to-do")
	return nil
}

func (dt DtWorkFlow) ProvisionDhcpIPV4Flow(subs *Subscriber) error {
	log.Info("dt-workflow-does-not-require-dhcp-ipv4-support--nothing-to-do")
	return nil
}

func (dt DtWorkFlow) ProvisionDhcpIPV6Flow(subs *Subscriber) error {
	log.Info("dt-workflow-does-not-require-dhcp-ipv6-support--nothing-to-do")
	return nil
}

func (dt DtWorkFlow) ProvisionIgmpFlow(subs *Subscriber) error {
	log.Info("dt-workflow-does-not-support-igmp-yet--nothing-to-do")
	return nil
}

func (dt DtWorkFlow) ProvisionHsiaFlow(subs *Subscriber) error {
	var err error
	var flowID uint32
	var gemPortIDs []uint32

	var allocID = subs.TpInstance[subs.TestConfig.TpIDList[0]].UsScheduler.AllocID
	for _, gem := range subs.TpInstance[subs.TestConfig.TpIDList[0]].UpstreamGemPortAttributeList {
		gemPortIDs = append(gemPortIDs, gem.GemportID)
	}

	for idx, gemID := range gemPortIDs {
		pBitMap := subs.TpInstance[subs.TestConfig.TpIDList[0]].UpstreamGemPortAttributeList[idx].PbitMap
		for pos, pbitSet := range strings.TrimPrefix(pBitMap, "0b") {
			if pbitSet == '1' {
				pcp := uint32(len(strings.TrimPrefix(pBitMap, "0b"))) - 1 - uint32(pos)
				if flowID, err = subs.RsrMgr.GetFlowID(context.Background(), uint32(subs.PonIntf)); err != nil {
					return errors.New(ReasonCodeToReasonString(FLOW_ID_GENERATION_FAILED))
				} else {
					if err := AddFlow(subs, HsiaFlow, Upstream, flowID, allocID, gemID, pcp); err != nil {
						return err
					}
					if err := AddFlow(subs, HsiaFlow, Downstream, flowID, allocID, gemID, pcp); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func (dt DtWorkFlow) ProvisionVoipFlow(subs *Subscriber) error {
	log.Info("dt-workflow-does-not-support-voip-yet--nothing-to-do")
	return nil
}

func (dt DtWorkFlow) ProvisionVodFlow(subs *Subscriber) error {
	log.Info("dt-workflow-does-not-support-vod-yet--nothing-to-do")
	return nil
}

func (dt DtWorkFlow) ProvisionMgmtFlow(subs *Subscriber) error {
	log.Info("dt-workflow-does-not-support-mgmt-yet--nothing-to-do")
	return nil
}

func (dt DtWorkFlow) ProvisionMulticastFlow(subs *Subscriber) error {
	log.Info("dt-workflow-does-not-support-multicast-yet--nothing-to-do")
	return nil
}
