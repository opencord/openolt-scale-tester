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
	"github.com/opencord/openolt-scale-tester/config"
	"github.com/opencord/voltha-lib-go/v2/pkg/log"
	"github.com/opencord/voltha-lib-go/v2/pkg/ponresourcemanager"
	oop "github.com/opencord/voltha-protos/v2/go/openolt"
	tp_pb "github.com/opencord/voltha-protos/v2/go/tech_profile"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func init() {
	_, _ = log.AddPackage(log.JSON, log.DebugLevel, nil)
}

// A dummy struct to comply with the WorkFlow interface.
type AttWorkFlow struct {
}

func ProvisionAttNniTrapFlow(oo oop.OpenoltClient, config *config.OpenOltScaleTesterConfig, rsrMgr *OpenOltResourceMgr) error {
	var flowID []uint32
	var err error

	if flowID, err = rsrMgr.ResourceMgrs[uint32(config.NniIntfID)].GetResourceID(uint32(config.NniIntfID),
		ponresourcemanager.FLOW_ID, 1); err != nil {
		return err
	}

	flowClassifier := &oop.Classifier{EthType: 2048, IpProto: 17, SrcPort: 67, DstPort: 68, PktTagType: "double_tag"}
	actionCmd := &oop.ActionCmd{TrapToHost: true}
	actionInfo := &oop.Action{Cmd: actionCmd}

	flow := oop.Flow{AccessIntfId: -1, OnuId: -1, UniId: -1, FlowId: flowID[0],
		FlowType: "downstream", AllocId: -1, GemportId: -1,
		Classifier: flowClassifier, Action: actionInfo,
		Priority: 1000, PortNo: 65536}

	_, err = oo.FlowAdd(context.Background(), &flow)

	st, _ := status.FromError(err)
	if st.Code() == codes.AlreadyExists {
		log.Debugw("Flow already exists", log.Fields{"err": err, "deviceFlow": flow})
		return nil
	}

	if err != nil {
		log.Errorw("Failed to Add flow to device", log.Fields{"err": err, "deviceFlow": flow})
		rsrMgr.ResourceMgrs[uint32(config.NniIntfID)].FreeResourceID(uint32(config.NniIntfID), ponresourcemanager.FLOW_ID, flowID)
		return err
	}
	log.Debugw("Flow added to device successfully ", log.Fields{"flow": flow})

	return nil
}

func getTrafficSched(subs *Subscriber, direction tp_pb.Direction) []*tp_pb.TrafficScheduler {
	var SchedCfg *tp_pb.SchedulerConfig

	if direction == tp_pb.Direction_DOWNSTREAM {
		SchedCfg = subs.RsrMgr.ResourceMgrs[subs.PonIntf].TechProfileMgr.
			GetDsScheduler(subs.TpInstance[subs.TestConfig.TpIDList[0]])

	} else {
		SchedCfg = subs.RsrMgr.ResourceMgrs[subs.PonIntf].TechProfileMgr.
			GetUsScheduler(subs.TpInstance[subs.TestConfig.TpIDList[0]])
	}

	// hard-code for now
	cir := 16000
	cbs := 5000
	eir := 16000
	ebs := 5000
	pir := cir + eir
	pbs := cbs + ebs

	TrafficShaping := &tp_pb.TrafficShapingInfo{Cir: uint32(cir), Cbs: uint32(cbs), Pir: uint32(pir), Pbs: uint32(pbs)}

	TrafficSched := []*tp_pb.TrafficScheduler{subs.RsrMgr.ResourceMgrs[subs.PonIntf].TechProfileMgr.
		GetTrafficScheduler(subs.TpInstance[subs.TestConfig.TpIDList[0]], SchedCfg, TrafficShaping)}

	return TrafficSched
}

func (att AttWorkFlow) ProvisionScheds(subs *Subscriber) error {
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

func getTrafficQueues(subs *Subscriber, direction tp_pb.Direction) []*tp_pb.TrafficQueue {

	trafficQueues := subs.RsrMgr.ResourceMgrs[subs.PonIntf].TechProfileMgr.
		GetTrafficQueues(subs.TpInstance[subs.TestConfig.TpIDList[0]], direction)

	return trafficQueues
}

func (att AttWorkFlow) ProvisionQueues(subs *Subscriber) error {
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

func (att AttWorkFlow) ProvisionEapFlow(subs *Subscriber) error {
	log.Info("provisioning-eap--att")

	var err error
	var flowID []uint32
	var gemPortIDs []uint32

	var allocID = subs.TpInstance[subs.TestConfig.TpIDList[0]].UsScheduler.AllocID

	for _, gem := range subs.TpInstance[subs.TestConfig.TpIDList[0]].UpstreamGemPortAttributeList {
		gemPortIDs = append(gemPortIDs, gem.GemportID)
	}

	for _, gemID := range gemPortIDs {
		if flowID, err = subs.RsrMgr.ResourceMgrs[uint32(subs.PonIntf)].GetResourceID(uint32(subs.PonIntf),
			ponresourcemanager.FLOW_ID, 1); err != nil {
			return errors.New(ReasonCodeToReasonString(FLOW_ADD_FAILED))
		}
		flowClassifier := &oop.Classifier{EthType: 34958, OVid: subs.Ctag, PktTagType: "single_tag"}
		actionCmd := &oop.ActionCmd{TrapToHost: true}
		actionInfo := &oop.Action{Cmd: actionCmd}

		flow := oop.Flow{AccessIntfId: int32(subs.PonIntf), OnuId: int32(subs.OnuID),
			UniId: int32(subs.UniID), FlowId: flowID[0],
			FlowType: "upstream", AllocId: int32(allocID), GemportId: int32(gemID),
			Classifier: flowClassifier, Action: actionInfo,
			Priority: 1000, PortNo: subs.UniPortNo}

		_, err = subs.OpenOltClient.FlowAdd(context.Background(), &flow)

		st, _ := status.FromError(err)
		if st.Code() == codes.AlreadyExists {
			log.Debugw("Flow already exists", log.Fields{"err": err, "deviceFlow": flow})
			continue
		}

		if err != nil {
			log.Errorw("Failed to Add flow to device", log.Fields{"err": err, "deviceFlow": flow})
			subs.RsrMgr.ResourceMgrs[uint32(subs.PonIntf)].FreeResourceID(uint32(subs.PonIntf),
				ponresourcemanager.FLOW_ID, flowID)
			return errors.New(ReasonCodeToReasonString(FLOW_ADD_FAILED))
		}
		log.Debugw("Flow added to device successfully ", log.Fields{"flow": flow})
	}
	return nil
}

func (att AttWorkFlow) ProvisionDhcpFlow(subs *Subscriber) error {
	// TODO
	log.Info("provisioning-dhcp")
	return nil
}

func (att AttWorkFlow) ProvisionIgmpFlow(subs *Subscriber) error {
	log.Info("att-workflow-does-not-support-igmp-yet--nothing-to-do")
	return nil
}

func (att AttWorkFlow) ProvisionHsiaFlow(subs *Subscriber) error {
	// TODO
	log.Info("provisioning-hsia")
	return nil
}
