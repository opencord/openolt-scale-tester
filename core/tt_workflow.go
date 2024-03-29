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
	"sync/atomic"

	"github.com/opencord/openolt-scale-tester/config"
	"github.com/opencord/voltha-lib-go/v7/pkg/log"
	oop "github.com/opencord/voltha-protos/v5/go/openolt"
	tp_pb "github.com/opencord/voltha-protos/v5/go/tech_profile"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var lastPonIntf *uint32 = new(uint32)

// A dummy struct to comply with the WorkFlow interface.
type TtWorkFlow struct {
}

func AddTtDhcpIPV4Flow(oo oop.OpenoltClient, config *config.OpenOltScaleTesterConfig, rsrMgr *OpenOltResourceMgr) error {
	var flowID uint64
	var err error

	// Allocating flowID from PON0 pool for an trap-from-nni flow
	if flowID, err = rsrMgr.GetFlowID(context.Background(), uint32(0)); err != nil {
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

func AddTtDhcpIPV6Flow(oo oop.OpenoltClient, config *config.OpenOltScaleTesterConfig, rsrMgr *OpenOltResourceMgr) error {
	logger.Info(nil, "tt-workflow-does-not-require-dhcp-ipv6-support--nothing-to-do")
	return nil
}

func ProvisionTtNniTrapFlow(oo oop.OpenoltClient, config *config.OpenOltScaleTesterConfig, rsrMgr *OpenOltResourceMgr) error {
	_ = AddTtDhcpIPV4Flow(oo, config, rsrMgr)
	return nil
}

func FormatTtClassfierAction(flowType string, direction string, subs *Subscriber) (oop.Classifier, oop.Action) {
	var flowClassifier oop.Classifier
	var actionCmd oop.ActionCmd
	var actionInfo oop.Action

	if direction == Upstream {
		switch flowType {
		case IgmpFlow:
			flowClassifier.EthType = IPv4EthType
			flowClassifier.IpProto = IgmpProto
			flowClassifier.SrcPort = 0
			flowClassifier.DstPort = 0
			flowClassifier.PktTagType = SingleTag
			actionCmd.TrapToHost = true
			actionInfo.Cmd = &actionCmd
		case HsiaFlow:
			actionCmd.AddOuterTag = true
			actionInfo.Cmd = &actionCmd
			actionInfo.IVid = 33
			actionInfo.OVid = 7
			flowClassifier.IPbits = 255
			flowClassifier.OVid = 33
			flowClassifier.PktTagType = SingleTag
		case VoipFlow:
			actionCmd.AddOuterTag = true
			actionInfo.Cmd = &actionCmd
			actionInfo.OPbits = 7
			actionInfo.IVid = 63
			actionInfo.OVid = 10
			flowClassifier.IPbits = 255
			flowClassifier.OPbits = 7
			flowClassifier.OVid = 63
			flowClassifier.PktTagType = SingleTag
		case VodFlow:
			actionCmd.AddOuterTag = true
			actionInfo.Cmd = &actionCmd
			actionInfo.OPbits = 5
			actionInfo.IVid = 55
			actionInfo.OVid = 555
			flowClassifier.IPbits = 255
			flowClassifier.OPbits = 5
			flowClassifier.OVid = 55
			flowClassifier.PktTagType = SingleTag
		case MgmtFlow:
			actionCmd.AddOuterTag = true
			actionInfo.Cmd = &actionCmd
			actionInfo.OPbits = 7
			actionInfo.IVid = 75
			actionInfo.OVid = 575
			flowClassifier.IPbits = 255
			flowClassifier.OPbits = 7
			flowClassifier.OVid = 75
			flowClassifier.PktTagType = SingleTag
		default:
			logger.Errorw(nil, "Unsupported TT flow type", log.Fields{"flowtype": flowType,
				"direction": direction})
		}
	} else if direction == Downstream {
		switch flowType {
		case IgmpFlow:
			logger.Errorw(nil, "Downstream IGMP flows are not required instead we have "+
				"IGMP trap flows already installed", log.Fields{"flowtype": flowType,
				"direction": direction})
		case HsiaFlow:
			actionCmd.RemoveOuterTag = true
			actionInfo.Cmd = &actionCmd
			actionInfo.IVid = 33
			flowClassifier.IPbits = 255
			flowClassifier.OPbits = 255
			flowClassifier.IVid = 33
			flowClassifier.OVid = 7
			flowClassifier.PktTagType = DoubleTag
		case VoipFlow:
			actionCmd.RemoveOuterTag = true
			actionInfo.Cmd = &actionCmd
			actionInfo.IPbits = 7
			actionInfo.IVid = 63
			flowClassifier.IPbits = 255
			flowClassifier.OPbits = 255
			flowClassifier.IVid = 63
			flowClassifier.OVid = 10
			flowClassifier.DstMac = GenerateMac(true)
			flowClassifier.PktTagType = DoubleTag
		case VodFlow:
			actionCmd.RemoveOuterTag = true
			actionInfo.Cmd = &actionCmd
			actionInfo.IPbits = 5
			actionInfo.IVid = 55
			flowClassifier.IPbits = 255
			flowClassifier.OPbits = 255
			flowClassifier.IVid = 55
			flowClassifier.OVid = 555
			flowClassifier.DstMac = GenerateMac(true)
			flowClassifier.PktTagType = DoubleTag
		case MgmtFlow:
			actionCmd.RemoveOuterTag = true
			actionInfo.Cmd = &actionCmd
			actionInfo.IPbits = 7
			actionInfo.IVid = 75
			flowClassifier.IPbits = 255
			flowClassifier.OPbits = 255
			flowClassifier.IVid = 75
			flowClassifier.OVid = 575
			flowClassifier.DstMac = GenerateMac(true)
			flowClassifier.PktTagType = DoubleTag
		default:
			logger.Errorw(nil, "Unsupported TT flow type", log.Fields{"flowtype": flowType,
				"direction": direction})
		}
	}
	return flowClassifier, actionInfo
}

func AddTtFlow(subs *Subscriber, flowType string, direction string, flowID uint64,
	allocID uint32, gemID uint32, pcp uint32, replicateFlow bool, symmetricFlowID uint64,
	pbitToGem map[uint32]uint32) error {

	var err error

	flowClassifier, actionInfo := FormatTtClassfierAction(flowType, direction, subs)
	// Update the o_pbit (if valid) for which this flow has to be classified
	if pcp != 0xff {
		flowClassifier.OPbits = pcp
	}
	flow := oop.Flow{AccessIntfId: int32(subs.PonIntf), OnuId: int32(subs.OnuID),
		UniId: int32(subs.UniID), FlowId: flowID,
		FlowType: direction, AllocId: int32(allocID), GemportId: int32(gemID),
		Classifier: &flowClassifier, Action: &actionInfo,
		Priority: 1000, PortNo: subs.UniPortNo,
		SymmetricFlowId: symmetricFlowID,
		ReplicateFlow:   replicateFlow, PbitToGemport: pbitToGem}

	logger.Infow(nil, "adding-flow", log.Fields{"flow": flow})

	_, err = subs.OpenOltClient.FlowAdd(context.Background(), &flow)

	st, _ := status.FromError(err)
	if st.Code() == codes.AlreadyExists {
		logger.Debugw(nil, "Flow already exists", log.Fields{"err": err, "deviceFlow": flow})
		return nil
	}

	if err != nil {
		logger.Errorw(nil, "Failed to Add flow to device", log.Fields{"err": err, "deviceFlow": flow})
		return errors.New(ReasonCodeToReasonString(FLOW_ADD_FAILED))
	}
	logger.Debugw(nil, "Flow added to device successfully ", log.Fields{"flow": flow})

	return nil
}

func (tt TtWorkFlow) ProvisionScheds(subs *Subscriber) error {
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

func (tt TtWorkFlow) ProvisionQueues(subs *Subscriber) error {
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

func (tt TtWorkFlow) ProvisionEapFlow(subs *Subscriber) error {
	logger.Info(nil, "tt-workflow-does-not-support-eap-yet--nothing-to-do")
	return nil
}

func (tt TtWorkFlow) ProvisionDhcpIPV4Flow(subs *Subscriber) error {
	logger.Info(nil, "tt-workflow-does-not-require-dhcp-ipv4-yet--nothing-to-do")
	return nil
}

func (tt TtWorkFlow) ProvisionDhcpIPV6Flow(subs *Subscriber) error {
	logger.Info(nil, "tt-workflow-does-not-require-dhcp-ipv6-support--nothing-to-do")
	return nil
}

func (tt TtWorkFlow) ProvisionIgmpFlow(subs *Subscriber) error {
	logger.Info(nil, "tt-workflow-does-not-require-igmp-support--nothing-to-do")
	return nil
}

func (tt TtWorkFlow) ProvisionHsiaFlow(subs *Subscriber) error {
	var gemPortIDs []uint32
	var err error
	var flowIDUs, flowIDDs uint64
	pbitToGem := make(map[uint32]uint32)
	var pcp uint32

	var allocID = subs.TpInstance[subs.TestConfig.TpIDList[0]].UsScheduler.AllocId
	for _, gem := range subs.TpInstance[subs.TestConfig.TpIDList[0]].UpstreamGemPortAttributeList {
		gemPortIDs = append(gemPortIDs, gem.GemportId)
	}

	for idx, gemID := range gemPortIDs {
		pBitMap := subs.TpInstance[subs.TestConfig.TpIDList[0]].UpstreamGemPortAttributeList[idx].PbitMap
		for pos, pbitSet := range strings.TrimPrefix(pBitMap, "0b") {
			if pbitSet == '1' {
				pcp = uint32(len(strings.TrimPrefix(pBitMap, "0b"))) - 1 - uint32(pos)
				var errUs, errDs error
				if flowIDUs, err = subs.RsrMgr.GetFlowID(context.Background(), subs.PonIntf); err != nil {
					return errors.New(ReasonCodeToReasonString(FLOW_ID_GENERATION_FAILED))
				}
				if errUs = AddTtFlow(subs, HsiaFlow, Upstream, flowIDUs, allocID, gemID, pcp, false,
					0, pbitToGem); errUs != nil {
					logger.Errorw(nil, "failed to install US HSIA flow",
						log.Fields{"onuID": subs.OnuID, "uniID": subs.UniID, "intf": subs.PonIntf})
				}
				if flowIDDs, err = subs.RsrMgr.GetFlowID(context.Background(), subs.PonIntf); err != nil {
					return errors.New(ReasonCodeToReasonString(FLOW_ID_GENERATION_FAILED))
				}
				if errDs = AddTtFlow(subs, HsiaFlow, Downstream, flowIDDs, allocID, gemID, pcp, false,
					flowIDUs, pbitToGem); errDs != nil {
					logger.Errorw(nil, "failed to install DS HSIA flow",
						log.Fields{"onuID": subs.OnuID, "uniID": subs.UniID, "intf": subs.PonIntf})
				}

				if errUs != nil || errDs != nil {
					if errUs != nil {
						return errUs
					}
					return errDs
				}
			}
		}
	}

	return nil
}

func (tt TtWorkFlow) ProvisionVoipFlow(subs *Subscriber) error {
	var err error
	var gemPortIDs []uint32
	var errUs, errDs, errDhcp error
	var flowIDUs, flowIDDs, flowIDDhcp uint64
	pbitToGem := make(map[uint32]uint32)
	var pcp uint32

	var allocID = subs.TpInstance[subs.TestConfig.TpIDList[0]].UsScheduler.AllocId
	for _, gem := range subs.TpInstance[subs.TestConfig.TpIDList[0]].UpstreamGemPortAttributeList {
		gemPortIDs = append(gemPortIDs, gem.GemportId)
	}

	for idx, gemID := range gemPortIDs {
		pBitMap := subs.TpInstance[subs.TestConfig.TpIDList[0]].UpstreamGemPortAttributeList[idx].PbitMap
		for pos, pbitSet := range strings.TrimPrefix(pBitMap, "0b") {
			if pbitSet == '1' {
				pcp = uint32(len(strings.TrimPrefix(pBitMap, "0b"))) - 1 - uint32(pos)
				if flowIDUs, err = subs.RsrMgr.GetFlowID(context.Background(), subs.PonIntf); err != nil {
					return errors.New(ReasonCodeToReasonString(FLOW_ID_GENERATION_FAILED))
				}
				if errUs = AddTtFlow(subs, VoipFlow, Upstream, flowIDUs, allocID, gemID, pcp, false,
					0, pbitToGem); errUs != nil {
					logger.Errorw(nil, "failed to install US VOIP flow",
						log.Fields{"onuID": subs.OnuID, "uniID": subs.UniID, "intf": subs.PonIntf})
				}
				if flowIDDs, err = subs.RsrMgr.GetFlowID(context.Background(), subs.PonIntf); err != nil {
					return errors.New(ReasonCodeToReasonString(FLOW_ID_GENERATION_FAILED))
				}
				if errDs = AddTtFlow(subs, VoipFlow, Downstream, flowIDDs, allocID, gemID, pcp, false,
					flowIDUs, pbitToGem); errDs != nil {
					logger.Errorw(nil, "failed to install DS VOIP flow",
						log.Fields{"onuID": subs.OnuID, "uniID": subs.UniID, "intf": subs.PonIntf})
				}
				if flowIDDhcp, err = subs.RsrMgr.GetFlowID(context.Background(), subs.PonIntf); err != nil {
					return errors.New(ReasonCodeToReasonString(FLOW_ID_GENERATION_FAILED))
				}
				if errDhcp = AddFlow(subs, DhcpFlowIPV4, Upstream, flowIDDhcp, allocID, gemID, pcp, false,
					0, pbitToGem); errDhcp != nil {
					logger.Errorw(nil, "failed to install US VOIP-DHCP flow",
						log.Fields{"onuID": subs.OnuID, "uniID": subs.UniID, "intf": subs.PonIntf})
				}

				if errUs != nil || errDs != nil || errDhcp != nil {
					if errUs != nil {
						return errUs
					}
					if errDs != nil {
						return errDs
					}
					return errDhcp
				}
			}
		}
	}

	return nil
}

func (tt TtWorkFlow) ProvisionVodFlow(subs *Subscriber) error {
	var err error
	var gemPortIDs []uint32
	var errUs, errDs, errDhcp, errIgmp error
	var flowIDUs, flowIDDs, flowIDDhcp, flowIDIgmp uint64
	pbitToGem := make(map[uint32]uint32)
	var pcp uint32

	var allocID = subs.TpInstance[subs.TestConfig.TpIDList[0]].UsScheduler.AllocId
	for _, gem := range subs.TpInstance[subs.TestConfig.TpIDList[0]].UpstreamGemPortAttributeList {
		gemPortIDs = append(gemPortIDs, gem.GemportId)
	}

	for idx, gemID := range gemPortIDs {
		pBitMap := subs.TpInstance[subs.TestConfig.TpIDList[0]].UpstreamGemPortAttributeList[idx].PbitMap
		for pos, pbitSet := range strings.TrimPrefix(pBitMap, "0b") {
			if pbitSet == '1' {
				pcp = uint32(len(strings.TrimPrefix(pBitMap, "0b"))) - 1 - uint32(pos)
				if flowIDUs, err = subs.RsrMgr.GetFlowID(context.Background(), subs.PonIntf); err != nil {
					return errors.New(ReasonCodeToReasonString(FLOW_ID_GENERATION_FAILED))
				}
				if errUs = AddTtFlow(subs, VodFlow, Upstream, flowIDUs, allocID, gemID, pcp, false,
					0, pbitToGem); errUs != nil {
					logger.Errorw(nil, "failed to install US VOIP flow",
						log.Fields{"onuID": subs.OnuID, "uniID": subs.UniID, "intf": subs.PonIntf})
				}
				if flowIDDs, err = subs.RsrMgr.GetFlowID(context.Background(), subs.PonIntf); err != nil {
					return errors.New(ReasonCodeToReasonString(FLOW_ID_GENERATION_FAILED))
				}
				if errDs = AddTtFlow(subs, VodFlow, Downstream, flowIDDs, allocID, gemID, pcp, false,
					flowIDUs, pbitToGem); errDs != nil {
					logger.Errorw(nil, "failed to install DS VOIP flow",
						log.Fields{"onuID": subs.OnuID, "uniID": subs.UniID, "intf": subs.PonIntf})
				}
				if flowIDDhcp, err = subs.RsrMgr.GetFlowID(context.Background(), subs.PonIntf); err != nil {
					return errors.New(ReasonCodeToReasonString(FLOW_ID_GENERATION_FAILED))
				}
				if errDhcp = AddFlow(subs, DhcpFlowIPV4, Upstream, flowIDDhcp, allocID, gemID, pcp, false,
					0, pbitToGem); errDhcp != nil {
					logger.Errorw(nil, "failed to install US VOIP-DHCP flow",
						log.Fields{"onuID": subs.OnuID, "uniID": subs.UniID, "intf": subs.PonIntf})
				}
				if flowIDIgmp, err = subs.RsrMgr.GetFlowID(context.Background(), subs.PonIntf); err != nil {
					return errors.New(ReasonCodeToReasonString(FLOW_ID_GENERATION_FAILED))
				}
				if errIgmp = AddTtFlow(subs, IgmpFlow, Upstream, flowIDIgmp, allocID, gemID, pcp, false,
					0, pbitToGem); errIgmp != nil {
					logger.Errorw(nil, "failed to install US VOIP-IGMP flow",
						log.Fields{"onuID": subs.OnuID, "uniID": subs.UniID, "intf": subs.PonIntf})
				}

				if errUs != nil || errDs != nil || errDhcp != nil || errIgmp != nil {
					if errUs != nil {
						return errUs
					}
					if errDs != nil {
						return errDs
					}
					if errDhcp != nil {
						return errDhcp
					}
					return errIgmp
				}
			}
		}
	}
	return nil
}

func (tt TtWorkFlow) ProvisionMgmtFlow(subs *Subscriber) error {
	var err error
	pbitToGem := make(map[uint32]uint32)
	var flowIDUs, flowIDDs, flowIDDhcp uint64
	var gemPortIDs []uint32
	var pcp uint32

	var allocID = subs.TpInstance[subs.TestConfig.TpIDList[0]].UsScheduler.AllocId
	for _, gem := range subs.TpInstance[subs.TestConfig.TpIDList[0]].UpstreamGemPortAttributeList {
		gemPortIDs = append(gemPortIDs, gem.GemportId)
	}

	for idx, gemID := range gemPortIDs {
		pBitMap := subs.TpInstance[subs.TestConfig.TpIDList[0]].UpstreamGemPortAttributeList[idx].PbitMap
		for pos, pbitSet := range strings.TrimPrefix(pBitMap, "0b") {
			if pbitSet == '1' {
				pcp = uint32(len(strings.TrimPrefix(pBitMap, "0b"))) - 1 - uint32(pos)
				var errUs, errDs, errDhcp error
				if flowIDUs, err = subs.RsrMgr.GetFlowID(context.Background(), subs.PonIntf); err != nil {
					return errors.New(ReasonCodeToReasonString(FLOW_ID_GENERATION_FAILED))
				}
				if errUs = AddTtFlow(subs, MgmtFlow, Upstream, flowIDUs, allocID, gemID, pcp, false,
					0, pbitToGem); errUs != nil {
					logger.Errorw(nil, "failed to install US MGMT flow",
						log.Fields{"onuID": subs.OnuID, "uniID": subs.UniID, "intf": subs.PonIntf})
				}
				if flowIDDs, err = subs.RsrMgr.GetFlowID(context.Background(), subs.PonIntf); err != nil {
					return errors.New(ReasonCodeToReasonString(FLOW_ID_GENERATION_FAILED))
				}
				if errDs = AddTtFlow(subs, MgmtFlow, Downstream, flowIDDs, allocID, gemID, pcp, false,
					flowIDUs, pbitToGem); errDs != nil {
					logger.Errorw(nil, "failed to install DS MGMT flow",
						log.Fields{"onuID": subs.OnuID, "uniID": subs.UniID, "intf": subs.PonIntf})
				}
				if flowIDDhcp, err = subs.RsrMgr.GetFlowID(context.Background(), subs.PonIntf); err != nil {
					return errors.New(ReasonCodeToReasonString(FLOW_ID_GENERATION_FAILED))
				}
				if errDhcp = AddFlow(subs, DhcpFlowIPV4, Upstream, flowIDDhcp, allocID, gemID, pcp, false,
					0, pbitToGem); errDhcp != nil {
					logger.Errorw(nil, "failed to install US MGMT-DHCP flow",
						log.Fields{"onuID": subs.OnuID, "uniID": subs.UniID, "intf": subs.PonIntf})
				}

				if errUs != nil || errDs != nil || errDhcp != nil {
					if errUs != nil {
						return errUs
					}
					if errDs != nil {
						return errDs
					}
					return errDhcp
				}
			}
		}
	}
	return nil
}

func (tt TtWorkFlow) ProvisionMulticastFlow(subs *Subscriber) error {
	var grp GroupData
	var err error

	numOfONUsPerPon := uint32(subs.TestConfig.NumOfOnu / uint(subs.RsrMgr.deviceInfo.GetPonPorts()))

	grp.Subs = *subs
	grp.Weight = 20
	grp.Priority = 0
	grp.OnuID = 6666
	grp.UniID = 6666
	grp.AllocID = 0
	grp.GemPortID = 4069
	grp.SchedPolicy = tp_pb.SchedulingPolicy_WRR

	logger.Debugw(nil, "Group data", log.Fields{"OnuID": subs.OnuID, "GroupID": grp.GroupID, "numOfONUsPerPon": numOfONUsPerPon})

	grp.GroupID = subs.OnuID

	if subs.PonIntf == 0 {
		grp.AddGroup = true
		grp.AddFlow = true
	} else {
		grp.AddFlow = false
		grp.AddGroup = false
	}

	if subs.PonIntf == atomic.LoadUint32(lastPonIntf) {
		_ = atomic.AddUint32(lastPonIntf, 1)
		grp.AddSched = true
		grp.AddQueue = true
	} else {
		grp.AddSched = false
		grp.AddQueue = false
	}

	grp.AddMember = true

	err = AddMulticastQueueFlow(&grp)

	if err != nil {
		logger.Errorw(nil, "Failed to add multicast flow", log.Fields{"error": err})
	}

	return err
}
