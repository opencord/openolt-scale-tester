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

const (
	DhcpIPProto = 17

	//Constants utilised while forming HSIA Flow
	HsiaFlow = "HSIA_FLOW"

	//Constants utilised while forming DHCP IPV4 Flow
	DhcpFlowIPV4    = "DHCP_FLOW_IPV4"
	IPv4EthType     = 0x800 //2048
	DhcpSrcPortIPV4 = 68
	DhcpDstPortIPV4 = 67

	//Constants utilised while forming DHCP IPV6 Flow
	DhcpFlowIPV6    = "DHCP_FLOW_IPV6"
	IPv6EthType     = 0x86dd //34525
	DhcpSrcPortIPV6 = 547
	DhcpDstPortIPV6 = 546

	//Constants utilised while forming EAPOL Flow
	EapolFlow  = "EAPOL_FLOW"
	EapEthType = 0x888e //34958

	//Direction constant
	Upstream   = "upstream"
	Downstream = "downstream"

	//PacketTagType constant
	PacketTagType = "pkt_tag_type"
	Untagged      = "untagged"
	SingleTag     = "single_tag"
	DoubleTag     = "double_tag"
)

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

func getTrafficQueues(subs *Subscriber, direction tp_pb.Direction) []*tp_pb.TrafficQueue {

	trafficQueues := subs.RsrMgr.ResourceMgrs[subs.PonIntf].TechProfileMgr.
		GetTrafficQueues(subs.TpInstance[subs.TestConfig.TpIDList[0]], direction)

	return trafficQueues
}

func FormatClassfierAction(flowType string, direction string, subs *Subscriber) (oop.Classifier, oop.Action) {
	var flowClassifier oop.Classifier
	var actionCmd oop.ActionCmd
	var actionInfo oop.Action

	if direction == Upstream {
		switch flowType {
		case EapolFlow:
			flowClassifier.EthType = EapEthType
			flowClassifier.OVid = subs.Ctag
			flowClassifier.PktTagType = SingleTag
			actionCmd.TrapToHost = true
			actionInfo.Cmd = &actionCmd
		case DhcpFlowIPV4:
			flowClassifier.EthType = IPv4EthType
			flowClassifier.IpProto = DhcpIPProto
			flowClassifier.SrcPort = DhcpSrcPortIPV4
			flowClassifier.DstPort = DhcpDstPortIPV4
			flowClassifier.PktTagType = SingleTag
			actionCmd.TrapToHost = true
			actionInfo.Cmd = &actionCmd
		case DhcpFlowIPV6:
			flowClassifier.EthType = IPv6EthType
			flowClassifier.IpProto = DhcpIPProto
			flowClassifier.SrcPort = DhcpSrcPortIPV6
			flowClassifier.DstPort = DhcpDstPortIPV6
			flowClassifier.PktTagType = SingleTag
			actionCmd.TrapToHost = true
			actionInfo.Cmd = &actionCmd
		case HsiaFlow:
			flowClassifier.OVid = subs.Ctag
			flowClassifier.PktTagType = SingleTag
			actionCmd.AddOuterTag = true
			actionInfo.Cmd = &actionCmd
			actionInfo.OVid = subs.Stag
		default:
			log.Errorw("Unsupported flow type", log.Fields{"flowtype": flowType,
				"direction": direction})
		}
	} else if direction == Downstream {
		switch flowType {
		case EapolFlow:
			log.Errorw("Downstream EAP flows are not required instead controller "+
				"packet outs EAP response directly to onu in downstream", log.Fields{"flowtype": flowType,
				"direction": direction})
		case DhcpFlowIPV4:
			log.Errorw("Downstream DHCPIPV4 flows are not required instead we have "+
				"NNI trap flows already installed", log.Fields{"flowtype": flowType,
				"direction": direction})
		case DhcpFlowIPV6:
			log.Errorw("Downstream DHCPIPV6 flows are not required instead we have "+
				"NNI trap flows already installed", log.Fields{"flowtype": flowType,
				"direction": direction})
		case HsiaFlow:
			flowClassifier.OVid = subs.Stag
			flowClassifier.IVid = subs.Ctag
			flowClassifier.PktTagType = DoubleTag
			actionCmd.RemoveOuterTag = true
			actionInfo.Cmd = &actionCmd
			actionInfo.OVid = subs.Stag
		default:
			log.Errorw("Unsupported flow type", log.Fields{"flowtype": flowType,
				"direction": direction})
		}
	}
	return flowClassifier, actionInfo
}

func AddFlow(subs *Subscriber, flowType string, direction string, flowID uint32,
	allocID uint32, gemID uint32, pcp uint32) error {
	log.Infow("add-flow", log.Fields{"WorkFlow": subs.TestConfig.WorkflowName, "FlowType": flowType,
		"direction": direction, "flowID": flowID})
	var err error

	flowClassifier, actionInfo := FormatClassfierAction(flowType, direction, subs)
	// Update the o_pbit for which this flow has to be classified
	flowClassifier.OPbits = pcp
	flow := oop.Flow{AccessIntfId: int32(subs.PonIntf), OnuId: int32(subs.OnuID),
		UniId: int32(subs.UniID), FlowId: flowID,
		FlowType: direction, AllocId: int32(allocID), GemportId: int32(gemID),
		Classifier: &flowClassifier, Action: &actionInfo,
		Priority: 1000, PortNo: subs.UniPortNo}

	_, err = subs.OpenOltClient.FlowAdd(context.Background(), &flow)

	st, _ := status.FromError(err)
	if st.Code() == codes.AlreadyExists {
		log.Debugw("Flow already exists", log.Fields{"err": err, "deviceFlow": flow})
		return nil
	}

	if err != nil {
		log.Errorw("Failed to Add flow to device", log.Fields{"err": err, "deviceFlow": flow})
		return errors.New(ReasonCodeToReasonString(FLOW_ADD_FAILED))
	}
	log.Debugw("Flow added to device successfully ", log.Fields{"flow": flow})

	return nil
}

func AddLldpFlow(oo oop.OpenoltClient, config *config.OpenOltScaleTesterConfig, rsrMgr *OpenOltResourceMgr) error {
	var flowID []uint32
	var err error

	if flowID, err = rsrMgr.ResourceMgrs[uint32(config.NniIntfID)].GetResourceID(uint32(config.NniIntfID),
		ponresourcemanager.FLOW_ID, 1); err != nil {
		return err
	}

	flowClassifier := &oop.Classifier{EthType: 35020, PktTagType: "untagged"}
	actionCmd := &oop.ActionCmd{TrapToHost: true}
	actionInfo := &oop.Action{Cmd: actionCmd}

	flow := oop.Flow{AccessIntfId: -1, OnuId: -1, UniId: -1, FlowId: flowID[0],
		FlowType: "downstream", AllocId: -1, GemportId: -1,
		Classifier: flowClassifier, Action: actionInfo,
		Priority: 1000, PortNo: uint32(config.NniIntfID)}

	_, err = oo.FlowAdd(context.Background(), &flow)

	st, _ := status.FromError(err)
	if st.Code() == codes.AlreadyExists {
		log.Debugw("Flow already exists", log.Fields{"err": err, "deviceFlow": flow})
		return nil
	}

	if err != nil {
		log.Errorw("Failed to Add LLDP flow to device", log.Fields{"err": err, "deviceFlow": flow})
		rsrMgr.ResourceMgrs[uint32(config.NniIntfID)].FreeResourceID(uint32(config.NniIntfID),
			ponresourcemanager.FLOW_ID, flowID)
		return err
	}
	log.Debugw("LLDP flow added to device successfully ", log.Fields{"flow": flow})

	return nil
}
