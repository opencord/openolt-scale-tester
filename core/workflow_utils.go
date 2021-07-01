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
	"math/rand"
	"time"

	"github.com/opencord/openolt-scale-tester/config"
	"github.com/opencord/voltha-lib-go/v7/pkg/log"
	oop "github.com/opencord/voltha-protos/v5/go/openolt"
	tp_pb "github.com/opencord/voltha-protos/v5/go/tech_profile"
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

	VoipFlow = "VOIP_FLOW"

	VodFlow = "VOD_FLOW"

	MgmtFlow = "MGMT_FLOW"

	IgmpProto = 2
	IgmpFlow  = "IGMP_FLOW"
)

const (
	MacSize = 6
	MacMin  = 0x0
	MacMax  = 0xFF
)

type GroupData struct {
	Subs        Subscriber             `json:"subscriber"`
	GroupID     uint32                 `json:"groupID"`
	Weight      uint32                 `json:"weight"`
	Priority    uint32                 `json:"priority"`
	OnuID       uint32                 `json:"onuID"`
	UniID       uint32                 `json:"uniID"`
	AllocID     uint32                 `json:"allocId"`
	GemPortID   uint32                 `json:"gemPortIds"`
	SchedPolicy tp_pb.SchedulingPolicy `json:"schedPolicy"`
	AddGroup    bool                   `json:"addGroup"`
	AddFlow     bool                   `json:"addFlow"`
	AddSched    bool                   `json:"addSched"`
	AddQueue    bool                   `json:"addQueue"`
	AddMember   bool                   `json:"addMember"`
}

func getTrafficSched(subs *Subscriber, direction tp_pb.Direction) []*tp_pb.TrafficScheduler {
	var SchedCfg *tp_pb.SchedulerConfig

	if direction == tp_pb.Direction_DOWNSTREAM {
		SchedCfg = subs.RsrMgr.TechprofileRef.
			GetDsScheduler(subs.TpInstance[subs.TestConfig.TpIDList[0]])
	} else {
		SchedCfg = subs.RsrMgr.TechprofileRef.
			GetUsScheduler(subs.TpInstance[subs.TestConfig.TpIDList[0]])
	}

	if SchedCfg == nil {
		logger.Errorw(nil, "Failed to create traffic schedulers", log.Fields{"direction": direction})
		return nil
	}

	// hard-code for now
	cir := 16000
	cbs := 5000
	eir := 16000
	ebs := 5000
	pir := cir + eir
	pbs := cbs + ebs

	TrafficShaping := &tp_pb.TrafficShapingInfo{Cir: uint32(cir), Cbs: uint32(cbs), Pir: uint32(pir), Pbs: uint32(pbs)}

	TrafficSched := []*tp_pb.TrafficScheduler{subs.RsrMgr.TechprofileRef.
		GetTrafficScheduler(subs.TpInstance[subs.TestConfig.TpIDList[0]], SchedCfg, TrafficShaping)}

	return TrafficSched
}

func getTrafficQueues(subs *Subscriber, direction tp_pb.Direction) []*tp_pb.TrafficQueue {

	trafficQueues, err := subs.RsrMgr.TechprofileRef.
		GetTrafficQueues(nil, subs.TpInstance[subs.TestConfig.TpIDList[0]], direction)

	if err == nil {
		return trafficQueues
	}

	logger.Errorw(nil, "Failed to create traffic queues", log.Fields{"direction": direction, "error": err})
	return nil
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
			logger.Errorw(nil, "Unsupported flow type", log.Fields{"flowtype": flowType,
				"direction": direction})
		}
	} else if direction == Downstream {
		switch flowType {
		case EapolFlow:
			logger.Errorw(nil, "Downstream EAP flows are not required instead controller "+
				"packet outs EAP response directly to onu in downstream", log.Fields{"flowtype": flowType,
				"direction": direction})
		case DhcpFlowIPV4:
			logger.Errorw(nil, "Downstream DHCPIPV4 flows are not required instead we have "+
				"NNI trap flows already installed", log.Fields{"flowtype": flowType,
				"direction": direction})
		case DhcpFlowIPV6:
			logger.Errorw(nil, "Downstream DHCPIPV6 flows are not required instead we have "+
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
			logger.Errorw(nil, "Unsupported flow type", log.Fields{"flowtype": flowType,
				"direction": direction})
		}
	}
	return flowClassifier, actionInfo
}

func AddFlow(subs *Subscriber, flowType string, direction string, flowID uint64,
	allocID uint32, gemID uint32, pcp uint32, replicateFlow bool, symmetricFlowID uint64,
	pbitToGem map[uint32]uint32) error {
	logger.Infow(nil, "add-flow", log.Fields{"WorkFlow": subs.TestConfig.WorkflowName, "FlowType": flowType,
		"direction": direction, "flowID": flowID})
	var err error

	flowClassifier, actionInfo := FormatClassfierAction(flowType, direction, subs)
	// Update the o_pbit (if valid) for which this flow has to be classified
	if pcp != 0xff {
		flowClassifier.OPbits = pcp
	}
	flow := oop.Flow{AccessIntfId: int32(subs.PonIntf), OnuId: int32(subs.OnuID),
		UniId: int32(subs.UniID), FlowId: flowID,
		FlowType: direction, AllocId: int32(allocID), GemportId: int32(gemID),
		Classifier: &flowClassifier, Action: &actionInfo,
		Priority: 1000, PortNo: subs.UniPortNo, SymmetricFlowId: symmetricFlowID,
		ReplicateFlow: replicateFlow, PbitToGemport: pbitToGem}

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

func AddLldpFlow(oo oop.OpenoltClient, config *config.OpenOltScaleTesterConfig, rsrMgr *OpenOltResourceMgr) error {
	var flowID uint64
	var err error

	if flowID, err = rsrMgr.GetFlowID(context.Background(), uint32(config.NniIntfID)); err != nil {
		return err
	}

	flowClassifier := &oop.Classifier{EthType: 35020, PktTagType: "untagged"}
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
		logger.Errorw(nil, "Failed to Add LLDP flow to device", log.Fields{"err": err, "deviceFlow": flow})
		return err
	}
	logger.Debugw(nil, "LLDP flow added to device successfully ", log.Fields{"flow": flow})

	return nil
}

func GenerateMac(isRand bool) []byte {
	var mac []byte

	if isRand {
		for i := 0; i < MacSize; i++ {
			mac = append(mac, byte(rand.Intn(MacMax-MacMin)+MacMin))
		}
	} else {
		mac = []byte{0x12, 0xAB, 0x34, 0xCD, 0x56, 0xEF}
	}

	return mac
}

func GenerateMulticastMac(onu_id uint32, group_id uint32) []byte {
	var mac []byte

	mac = []byte{0x01, 0x00, 0x5E}

	mac = append(mac, byte(onu_id%255))
	mac = append(mac, byte(rand.Intn(MacMax-MacMin)+MacMin))
	mac = append(mac, byte(group_id))

	return mac
}

func PerformGroupOperation(grp *GroupData, groupCfg *oop.Group) (*oop.Empty, error) {
	oo := grp.Subs.OpenOltClient

	var err error
	var res *oop.Empty

	if res, err = oop.OpenoltClient.PerformGroupOperation(oo, context.Background(), groupCfg); err != nil {
		logger.Errorw(nil, "Failed to perform - PerformGroupOperation()", log.Fields{"err": err})
		return nil, err
	}

	logger.Info(nil, "Successfully called - PerformGroupOperation()")

	return res, nil
}

func CreateGroup(grp *GroupData) (*oop.Empty, error) {
	var groupCfg oop.Group

	logger.Infow(nil, "creating group", log.Fields{"GroupID": grp.GroupID})

	groupCfg.Command = oop.Group_SET_MEMBERS
	groupCfg.GroupId = grp.GroupID

	return PerformGroupOperation(grp, &groupCfg)
}

func OpMulticastTrafficQueue(grp *GroupData, isCreating bool) (*oop.Empty, error) {
	logger.Infow(nil, "operating on multicast traffic queue", log.Fields{"Creating": isCreating, "GroupID": grp.GroupID})

	oo := grp.Subs.OpenOltClient

	var request tp_pb.TrafficQueues
	request.IntfId = grp.Subs.PonIntf
	request.OnuId = grp.Subs.OnuID
	request.UniId = grp.Subs.UniID

	var trafficQueues []*tp_pb.TrafficQueue

	var trafficQueue tp_pb.TrafficQueue
	trafficQueue.Direction = tp_pb.Direction_DOWNSTREAM
	trafficQueue.Priority = grp.Priority
	trafficQueue.Weight = grp.Weight
	trafficQueue.GemportId = grp.GemPortID
	trafficQueue.SchedPolicy = grp.SchedPolicy

	trafficQueues = append(trafficQueues, &trafficQueue)

	request.TrafficQueues = trafficQueues

	var err error
	var res *oop.Empty

	if isCreating {
		if res, err = oop.OpenoltClient.CreateTrafficQueues(oo, context.Background(), &request); err != nil {
			logger.Errorw(nil, "Failed to perform - CreateTrafficQueues()", log.Fields{"err": err})
			return nil, err
		}

		logger.Info(nil, "Successfully called - CreateTrafficQueues()")
	} else {
		if res, err = oop.OpenoltClient.RemoveTrafficQueues(oo, context.Background(), &request); err != nil {
			logger.Errorw(nil, "Failed to perform - RemoveTrafficQueues()", log.Fields{"err": err})
			return nil, err
		}

		logger.Info(nil, "Successfully called - RemoveTrafficQueues()")
	}

	return res, nil
}

func AddMulticastFlow(grp *GroupData) error {
	logger.Infow(nil, "add multicast flow", log.Fields{"GroupID": grp.GroupID})

	oo := grp.Subs.OpenOltClient
	config := grp.Subs.TestConfig
	rsrMgr := grp.Subs.RsrMgr

	var flowID uint64
	var err error

	if flowID, err = rsrMgr.GetFlowID(context.Background(), uint32(config.NniIntfID)); err != nil {
		return err
	}

	flowClassifier := &oop.Classifier{
		IPbits:     255,
		OPbits:     255,
		IVid:       55,
		OVid:       255,
		DstMac:     GenerateMulticastMac(grp.Subs.OnuID, grp.GroupID),
		PktTagType: DoubleTag}

	flow := oop.Flow{AccessIntfId: int32(grp.Subs.PonIntf), OnuId: int32(grp.Subs.OnuID), UniId: int32(grp.Subs.UniID), FlowId: flowID,
		FlowType: "multicast", AllocId: int32(grp.AllocID), GemportId: int32(grp.GemPortID),
		Classifier: flowClassifier, Priority: int32(grp.Priority), PortNo: uint32(grp.Subs.UniPortNo), GroupId: uint32(grp.GroupID)}

	_, err = oo.FlowAdd(context.Background(), &flow)

	st, _ := status.FromError(err)
	if st.Code() == codes.AlreadyExists {
		logger.Debugw(nil, "Flow already exists", log.Fields{"err": err, "deviceFlow": flow})
		return nil
	}

	if err != nil {
		logger.Errorw(nil, "Failed to add multicast flow to device", log.Fields{"err": err, "deviceFlow": flow})
		return err
	}

	logger.Debugw(nil, "Multicast flow added to device successfully ", log.Fields{"flow": flow})

	return nil
}

func AddMulticastSched(grp *GroupData) error {
	logger.Infow(nil, "creating multicast sched", log.Fields{"GroupID": grp.GroupID})

	SchedCfg := &tp_pb.SchedulerConfig{
		Direction:    tp_pb.Direction_DOWNSTREAM,
		AdditionalBw: tp_pb.AdditionalBW_AdditionalBW_BestEffort,
		Priority:     grp.Priority,
		Weight:       grp.Weight,
		SchedPolicy:  tp_pb.SchedulingPolicy_WRR}

	// hard-code for now
	cir := 1948
	cbs := 31768
	eir := 100
	ebs := 1000
	pir := cir + eir
	pbs := cbs + ebs

	TfShInfo := &tp_pb.TrafficShapingInfo{Cir: uint32(cir), Cbs: uint32(cbs), Pir: uint32(pir), Pbs: uint32(pbs)}

	TrafficSched := []*tp_pb.TrafficScheduler{grp.Subs.RsrMgr.TechprofileRef.
		GetTrafficScheduler(grp.Subs.TpInstance[grp.Subs.TestConfig.TpIDList[0]], SchedCfg, TfShInfo)}

	if TrafficSched == nil {
		logger.Error(nil, "Create scheduler for multicast traffic failed")
		return errors.New(ReasonCodeToReasonString(SCHED_CREATION_FAILED))
	}

	logger.Debugw(nil, "Sending Traffic scheduler create to device",
		log.Fields{"Direction": tp_pb.Direction_DOWNSTREAM, "TrafficScheds": TrafficSched})

	if _, err := grp.Subs.OpenOltClient.CreateTrafficSchedulers(context.Background(), &tp_pb.TrafficSchedulers{
		IntfId: grp.Subs.PonIntf, OnuId: grp.Subs.OnuID,
		UniId: grp.Subs.UniID, PortNo: grp.Subs.UniPortNo,
		TrafficScheds: TrafficSched}); err != nil {
		logger.Errorw(nil, "Failed to create traffic schedulers", log.Fields{"error": err})
		return errors.New(ReasonCodeToReasonString(SCHED_CREATION_FAILED))
	}

	return nil
}

func OpMemberToGroup(grp *GroupData, isAdding bool) (*oop.Empty, error) {
	logger.Infow(nil, "operating on group", log.Fields{"Adding": isAdding})

	var groupCfg oop.Group

	if isAdding {
		groupCfg.Command = oop.Group_ADD_MEMBERS
	} else {
		groupCfg.Command = oop.Group_REMOVE_MEMBERS
	}

	groupCfg.GroupId = grp.GroupID

	var members []*oop.GroupMember

	var member0 oop.GroupMember
	member0.InterfaceId = grp.Subs.PonIntf
	member0.GemPortId = grp.GemPortID
	member0.Priority = grp.Priority
	//member0.SchedPolicy = tp_pb.SchedulingPolicy_WRR
	member0.InterfaceType = oop.GroupMember_PON

	members = append(members, &member0)

	groupCfg.Members = members

	return PerformGroupOperation(grp, &groupCfg)
}

func AddMulticastQueueFlow(grp *GroupData) error {
	var err error

	logger.Debugw(nil, "Create multicast queue flow", log.Fields{"GroupID": grp.GroupID, "AddGroup": grp.AddGroup,
		"AddFlow": grp.AddFlow, "AddSched": grp.AddSched, "AddQueue": grp.AddQueue, "AddMember": grp.AddMember})

	if grp.AddGroup {
		if _, err = CreateGroup(grp); err != nil {
			logger.Error(nil, "Failed to add group to device")
			return err
		}
	}

	if grp.AddFlow {
		if err = AddMulticastFlow(grp); err != nil {
			logger.Error(nil, "Failed to add multicast flow to device")
			return err
		}
	}

	if grp.AddSched {
		if err = AddMulticastSched(grp); err != nil {
			logger.Error(nil, "Failed to add multicast sched to device")
			return err
		}
	}

	if grp.AddQueue {
		if _, err = OpMulticastTrafficQueue(grp, true); err != nil {
			logger.Error(nil, "Failed to add multicast queue to device")
			return err
		}
	}

	if grp.AddMember {
		if _, err = OpMemberToGroup(grp, true); err != nil {
			logger.Error(nil, "Failed to add member to group")
			return err
		}
	}

	return nil
}

func CreateTrafficSchedWithRetry(OpenOltClient oop.OpenoltClient, sched *oop.TrafficSchedulers) error {
	maxRetry := 20
	if _, err := OpenOltClient.CreateTrafficSchedulers(context.Background(), sched); err == nil {
		logger.Info(nil, "succeeded in first attempt")
		return nil
	} else {
		logger.Info(nil, "going for a retry")
	}
	for i := 0; i < maxRetry; i++ {
		if _, err := OpenOltClient.CreateTrafficSchedulers(context.Background(), sched); err != nil {
			logger.Error(nil, "retying after delay")
			time.Sleep(50 * time.Millisecond)
			continue
		} else {
			logger.Infow(nil, "succeeded in retry iteration=%d!!", log.Fields{"i": i})
			return nil
		}
	}

	return errors.New("failed-to-create-traffic-sched-after-all-retries")
}

func CreateTrafficQueuesWithRetry(OpenOltClient oop.OpenoltClient, queue *oop.TrafficQueues) error {
	maxRetry := 20
	if _, err := OpenOltClient.CreateTrafficQueues(context.Background(), queue); err == nil {
		logger.Info(nil, "succeeded in first attempt")
		return nil
	}
	for i := 0; i < maxRetry; i++ {
		if _, err := OpenOltClient.CreateTrafficQueues(context.Background(), queue); err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		} else {
			logger.Infow(nil, "succeeded in retry iteration=%d!!", log.Fields{"i": i})
			return nil
		}
	}

	return errors.New("failed-to-create-traffic-queue-after-all-retries")
}

func AddFlowWithRetry(OpenOltClient oop.OpenoltClient, flow *oop.Flow) error {

	var err error
	maxRetry := 20

	_, err = OpenOltClient.FlowAdd(context.Background(), flow)

	st, _ := status.FromError(err)
	if st.Code() == codes.AlreadyExists {
		logger.Debugw(nil, "Flow already exists", log.Fields{"err": err, "deviceFlow": flow})
		return nil
	}
	if st.Code() == codes.ResourceExhausted {
		for i := 0; i < maxRetry; i++ {
			_, err = OpenOltClient.FlowAdd(context.Background(), flow)
			st, _ := status.FromError(err)
			if st.Code() == codes.ResourceExhausted {
				logger.Error(nil, "flow-install-failed--retrying")
				continue
			} else if st.Code() == codes.OK {
				logger.Infow(nil, "flow-install-succeeded-on-retry", log.Fields{"i": i, "flow": flow})
				return nil
			}
		}

	}

	logger.Debugw(nil, "Flow install failed on all retries ", log.Fields{"flow": flow})

	return err
}
