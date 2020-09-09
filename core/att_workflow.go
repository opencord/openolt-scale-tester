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
	"github.com/opencord/voltha-lib-go/v4/pkg/log"
	oop "github.com/opencord/voltha-protos/v4/go/openolt"
	tp_pb "github.com/opencord/voltha-protos/v4/go/tech_profile"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// A dummy struct to comply with the WorkFlow interface.
type AttWorkFlow struct {
}

func AddDhcpIPV4Flow(oo oop.OpenoltClient, config *config.OpenOltScaleTesterConfig, rsrMgr *OpenOltResourceMgr) error {
	var flowID uint64
	var err error

	if flowID, err = rsrMgr.GetFlowID(context.Background(), uint32(config.NniIntfID)); err != nil {
		return err
	}

	// DHCP IPV4
	flowClassifier := &oop.Classifier{EthType: 2048, IpProto: 17, SrcPort: 67, DstPort: 68, PktTagType: "double_tag"}
	actionCmd := &oop.ActionCmd{TrapToHost: true}
	actionInfo := &oop.Action{Cmd: actionCmd}

	flow := oop.Flow{AccessIntfId: -1, OnuId: -1, UniId: -1, FlowId: flowID,
		FlowType: "downstream", AllocId: -1, GemportId: -1,
		Classifier: flowClassifier, Action: actionInfo,
		Priority: 1000, PortNo: uint32(config.NniIntfID)}

	_, err = oo.FlowAdd(context.Background(), &flow)

	st, _ := status.FromError(err)
	if st.Code() == codes.AlreadyExists {
		logger.Debugw(nil, "Flow already exists", log.Fields{"err": err, "deviceFlow": flow})
		return nil
	}

	if err != nil {
		logger.Errorw(nil, "Failed to Add DHCP IPv4 to device", log.Fields{"err": err, "deviceFlow": flow})
		return err
	}
	logger.Debugw(nil, "DHCP IPV4 added to device successfully ", log.Fields{"flow": flow})

	return nil
}

func AddDhcpIPV6Flow(oo oop.OpenoltClient, config *config.OpenOltScaleTesterConfig, rsrMgr *OpenOltResourceMgr) error {
	var flowID uint64
	var err error

	if flowID, err = rsrMgr.GetFlowID(context.Background(), uint32(config.NniIntfID)); err != nil {
		return err
	}

	// DHCP IPV6
	flowClassifier := &oop.Classifier{EthType: 34525, IpProto: 17, SrcPort: 546, DstPort: 547, PktTagType: "double_tag"}
	actionCmd := &oop.ActionCmd{TrapToHost: true}
	actionInfo := &oop.Action{Cmd: actionCmd}

	flow := oop.Flow{AccessIntfId: -1, OnuId: -1, UniId: -1, FlowId: flowID,
		FlowType: "downstream", AllocId: -1, GemportId: -1,
		Classifier: flowClassifier, Action: actionInfo,
		Priority: 1000, PortNo: uint32(config.NniIntfID)}

	_, err = oo.FlowAdd(context.Background(), &flow)

	st, _ := status.FromError(err)
	if st.Code() == codes.AlreadyExists {
		logger.Debugw(nil, "Flow already exists", log.Fields{"err": err, "deviceFlow": flow})
		return nil
	}

	if err != nil {
		logger.Errorw(nil, "Failed to Add DHCP IPV6 to device", log.Fields{"err": err, "deviceFlow": flow})
		return err
	}
	logger.Debugw(nil, "DHCP IPV6 added to device successfully ", log.Fields{"flow": flow})

	return nil
}

func ProvisionAttNniTrapFlow(oo oop.OpenoltClient, config *config.OpenOltScaleTesterConfig, rsrMgr *OpenOltResourceMgr) error {
	_ = AddDhcpIPV4Flow(oo, config, rsrMgr)
	_ = AddDhcpIPV6Flow(oo, config, rsrMgr)
	_ = AddLldpFlow(oo, config, rsrMgr)

	return nil
}

func (att AttWorkFlow) ProvisionScheds(subs *Subscriber) error {
	var trafficSched []*tp_pb.TrafficScheduler

	logger.Info(nil, "provisioning-scheds")

	if trafficSched = getTrafficSched(subs, tp_pb.Direction_DOWNSTREAM); trafficSched == nil {
		logger.Error(nil, "ds-traffic-sched-is-nil")
		return errors.New(ReasonCodeToReasonString(SCHED_CREATION_FAILED))
	}

	logger.Debugw(nil, "Sending Traffic scheduler create to device",
		log.Fields{"Direction": tp_pb.Direction_DOWNSTREAM, "TrafficScheds": trafficSched})
	if _, err := subs.OpenOltClient.CreateTrafficSchedulers(context.Background(), &tp_pb.TrafficSchedulers{
		IntfId: subs.PonIntf, OnuId: subs.OnuID,
		UniId: subs.UniID, PortNo: subs.UniPortNo,
		TrafficScheds: trafficSched}); err != nil {
		logger.Errorw(nil, "Failed to create traffic schedulers", log.Fields{"error": err})
		return errors.New(ReasonCodeToReasonString(SCHED_CREATION_FAILED))
	}

	if trafficSched = getTrafficSched(subs, tp_pb.Direction_UPSTREAM); trafficSched == nil {
		logger.Error(nil, "us-traffic-sched-is-nil")
		return errors.New(ReasonCodeToReasonString(SCHED_CREATION_FAILED))
	}

	logger.Debugw(nil, "Sending Traffic scheduler create to device",
		log.Fields{"Direction": tp_pb.Direction_UPSTREAM, "TrafficScheds": trafficSched})
	if _, err := subs.OpenOltClient.CreateTrafficSchedulers(context.Background(), &tp_pb.TrafficSchedulers{
		IntfId: subs.PonIntf, OnuId: subs.OnuID,
		UniId: subs.UniID, PortNo: subs.UniPortNo,
		TrafficScheds: trafficSched}); err != nil {
		logger.Errorw(nil, "Failed to create traffic schedulers", log.Fields{"error": err})
		return errors.New(ReasonCodeToReasonString(SCHED_CREATION_FAILED))
	}
	return nil
}

func (att AttWorkFlow) ProvisionQueues(subs *Subscriber) error {
	logger.Info(nil, "provisioning-queues")

	var trafficQueues []*tp_pb.TrafficQueue
	if trafficQueues = getTrafficQueues(subs, tp_pb.Direction_DOWNSTREAM); trafficQueues == nil {
		logger.Error(nil, "Failed to create traffic queues")
		return errors.New(ReasonCodeToReasonString(QUEUE_CREATION_FAILED))
	}

	// On receiving the CreateTrafficQueues request, the driver should create corresponding
	// downstream queues.
	logger.Debugw(nil, "Sending Traffic Queues create to device",
		log.Fields{"Direction": tp_pb.Direction_DOWNSTREAM, "TrafficQueues": trafficQueues})
	if _, err := subs.OpenOltClient.CreateTrafficQueues(context.Background(),
		&tp_pb.TrafficQueues{IntfId: subs.PonIntf, OnuId: subs.OnuID,
			UniId: subs.UniID, PortNo: subs.UniPortNo,
			TrafficQueues: trafficQueues}); err != nil {
		logger.Errorw(nil, "Failed to create traffic queues in device", log.Fields{"error": err})
		return errors.New(ReasonCodeToReasonString(QUEUE_CREATION_FAILED))
	}

	if trafficQueues = getTrafficQueues(subs, tp_pb.Direction_UPSTREAM); trafficQueues == nil {
		logger.Error(nil, "Failed to create traffic queues")
		return errors.New(ReasonCodeToReasonString(QUEUE_CREATION_FAILED))
	}

	// On receiving the CreateTrafficQueues request, the driver should create corresponding
	// upstream queues.
	logger.Debugw(nil, "Sending Traffic Queues create to device",
		log.Fields{"Direction": tp_pb.Direction_UPSTREAM, "TrafficQueues": trafficQueues})
	if _, err := subs.OpenOltClient.CreateTrafficQueues(context.Background(),
		&tp_pb.TrafficQueues{IntfId: subs.PonIntf, OnuId: subs.OnuID,
			UniId: subs.UniID, PortNo: subs.UniPortNo,
			TrafficQueues: trafficQueues}); err != nil {
		logger.Errorw(nil, "Failed to create traffic queues in device", log.Fields{"error": err})
		return errors.New(ReasonCodeToReasonString(QUEUE_CREATION_FAILED))
	}

	return nil
}

func (att AttWorkFlow) ProvisionEapFlow(subs *Subscriber) error {
	var err error
	var flowID uint64
	var gemPortIDs []uint32
	pbitToGem := make(map[uint32]uint32)

	var allocID = subs.TpInstance[subs.TestConfig.TpIDList[0]].UsScheduler.AllocID
	for _, gem := range subs.TpInstance[subs.TestConfig.TpIDList[0]].UpstreamGemPortAttributeList {
		gemPortIDs = append(gemPortIDs, gem.GemportID)
	}

	for idx, gemID := range gemPortIDs {
		pBitMap := subs.TpInstance[subs.TestConfig.TpIDList[0]].UpstreamGemPortAttributeList[idx].PbitMap
		for pos, pbitSet := range strings.TrimPrefix(pBitMap, "0b") {
			if pbitSet == '1' {
				pcp := uint32(len(strings.TrimPrefix(pBitMap, "0b"))) - 1 - uint32(pos)
				pbitToGem[pcp] = gemID
			}
		}
	}
	// This flowID is not the BAL flow ID now, it is the voltha-flow-id
	if flowID, err = subs.RsrMgr.GetFlowID(context.Background(), uint32(subs.PonIntf)); err != nil {
		return errors.New(ReasonCodeToReasonString(FLOW_ID_GENERATION_FAILED))
	}
	if err := AddFlow(subs, EapolFlow, Upstream, flowID, allocID, 0, 0xff,
		true, 0, pbitToGem); err != nil {
		return err
	}

	return nil
}

func (att AttWorkFlow) ProvisionDhcpIPV4Flow(subs *Subscriber) error {
	var err error
	var flowID uint64
	var gemPortIDs []uint32
	pbitToGem := make(map[uint32]uint32)

	var allocID = subs.TpInstance[subs.TestConfig.TpIDList[0]].UsScheduler.AllocID
	for _, gem := range subs.TpInstance[subs.TestConfig.TpIDList[0]].UpstreamGemPortAttributeList {
		gemPortIDs = append(gemPortIDs, gem.GemportID)
	}

	for idx, gemID := range gemPortIDs {
		pBitMap := subs.TpInstance[subs.TestConfig.TpIDList[0]].UpstreamGemPortAttributeList[idx].PbitMap
		for pos, pbitSet := range strings.TrimPrefix(pBitMap, "0b") {
			if pbitSet == '1' {
				pcp := uint32(len(strings.TrimPrefix(pBitMap, "0b"))) - 1 - uint32(pos)
				pbitToGem[pcp] = gemID
			}
		}
	}

	// This flowID is not the BAL flow ID now, it is the voltha-flow-id
	if flowID, err = subs.RsrMgr.GetFlowID(context.Background(), uint32(subs.PonIntf)); err != nil {
		return errors.New(ReasonCodeToReasonString(FLOW_ID_GENERATION_FAILED))
	}
	if err := AddFlow(subs, DhcpFlowIPV4, Upstream, flowID, allocID, 0, 0xff,
		true, 0, pbitToGem); err != nil {
		return err
	}
	return nil
}

func (att AttWorkFlow) ProvisionDhcpIPV6Flow(subs *Subscriber) error {
	var err error
	var flowID uint64
	var gemPortIDs []uint32
	pbitToGem := make(map[uint32]uint32)

	var allocID = subs.TpInstance[subs.TestConfig.TpIDList[0]].UsScheduler.AllocID
	for _, gem := range subs.TpInstance[subs.TestConfig.TpIDList[0]].UpstreamGemPortAttributeList {
		gemPortIDs = append(gemPortIDs, gem.GemportID)
	}

	for idx, gemID := range gemPortIDs {
		pBitMap := subs.TpInstance[subs.TestConfig.TpIDList[0]].UpstreamGemPortAttributeList[idx].PbitMap
		for pos, pbitSet := range strings.TrimPrefix(pBitMap, "0b") {
			if pbitSet == '1' {
				pcp := uint32(len(strings.TrimPrefix(pBitMap, "0b"))) - 1 - uint32(pos)
				pbitToGem[pcp] = gemID
			}
		}
	}

	// This flowID is not the BAL flow ID now, it is the voltha-flow-id
	if flowID, err = subs.RsrMgr.GetFlowID(context.Background(), uint32(subs.PonIntf)); err != nil {
		return errors.New(ReasonCodeToReasonString(FLOW_ID_GENERATION_FAILED))
	}
	if err := AddFlow(subs, DhcpFlowIPV6, Upstream, flowID, allocID, 0, 0xff,
		true, 0, pbitToGem); err != nil {
		return err
	}
	return nil
}

func (att AttWorkFlow) ProvisionIgmpFlow(subs *Subscriber) error {
	logger.Info(nil, "att-workflow-does-not-support-igmp-yet--nothing-to-do")
	return nil
}

func (att AttWorkFlow) ProvisionHsiaFlow(subs *Subscriber) error {
	var err error
	var flowIDUs, flowIDDs uint64
	var gemPortIDs []uint32
	pbitToGem := make(map[uint32]uint32)

	var allocID = subs.TpInstance[subs.TestConfig.TpIDList[0]].UsScheduler.AllocID
	for _, gem := range subs.TpInstance[subs.TestConfig.TpIDList[0]].UpstreamGemPortAttributeList {
		gemPortIDs = append(gemPortIDs, gem.GemportID)
	}

	for idx, gemID := range gemPortIDs {
		pBitMap := subs.TpInstance[subs.TestConfig.TpIDList[0]].UpstreamGemPortAttributeList[idx].PbitMap
		for pos, pbitSet := range strings.TrimPrefix(pBitMap, "0b") {
			if pbitSet == '1' {
				pcp := uint32(len(strings.TrimPrefix(pBitMap, "0b"))) - 1 - uint32(pos)
				pbitToGem[pcp] = gemID
			}
		}
	}

	// This flowID is not the BAL flow ID now, it is the voltha-flow-id
	if flowIDUs, err = subs.RsrMgr.GetFlowID(context.Background(), uint32(subs.PonIntf)); err != nil {
		return errors.New(ReasonCodeToReasonString(FLOW_ID_GENERATION_FAILED))
	}
	if err := AddFlow(subs, HsiaFlow, Upstream, flowIDUs, allocID, 0, 0xff,
		true, 0, pbitToGem); err != nil {
		return err
	}
	if flowIDDs, err = subs.RsrMgr.GetFlowID(context.Background(), uint32(subs.PonIntf)); err != nil {
		return errors.New(ReasonCodeToReasonString(FLOW_ID_GENERATION_FAILED))
	}
	if err := AddFlow(subs, HsiaFlow, Downstream, flowIDDs, allocID, 0, 0xff,
		true, flowIDUs, pbitToGem); err != nil {
		return err
	}
	return nil
}

func (att AttWorkFlow) ProvisionVoipFlow(subs *Subscriber) error {
	logger.Info(nil, "att-workflow-does-not-support-voip-yet--nothing-to-do")
	return nil
}

func (att AttWorkFlow) ProvisionVodFlow(subs *Subscriber) error {
	logger.Info(nil, "att-workflow-does-not-support-vod-yet--nothing-to-do")
	return nil
}

func (att AttWorkFlow) ProvisionMgmtFlow(subs *Subscriber) error {
	logger.Info(nil, "att-workflow-does-not-support-mgmt-yet--nothing-to-do")
	return nil
}

func (att AttWorkFlow) ProvisionMulticastFlow(subs *Subscriber) error {
	logger.Info(nil, "att-workflow-does-not-support-multicast-yet--nothing-to-do")
	return nil
}
