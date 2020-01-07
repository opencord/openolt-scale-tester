/*
 * Copyright 2019-present Open Networking Foundation

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

//Package resourcemanager provides the utility for managing resources
package core

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/opencord/voltha-lib-go/v2/pkg/log"
	ponrmgr "github.com/opencord/voltha-lib-go/v2/pkg/ponresourcemanager"
	"github.com/opencord/voltha-protos/v2/go/openolt"
)

func init() {
	_, _ = log.AddPackage(log.JSON, log.DebugLevel, nil)
}

// OpenOltResourceMgr holds resource related information as provided below for each field
type OpenOltResourceMgr struct {
	deviceInfo *openolt.DeviceInfo
	// array of pon resource managers per interface technology
	ResourceMgrs map[uint32]*ponrmgr.PONResourceManager
}

// NewResourceMgr init a New resource manager instance which in turn instantiates pon resource manager
// instances according to technology. Initializes the default resource ranges for all
// the resources.
func NewResourceMgr(deviceID string, KVStoreHostPort string, kvStoreType string, deviceType string, devInfo *openolt.DeviceInfo) *OpenOltResourceMgr {
	var ResourceMgr OpenOltResourceMgr
	log.Debugf("Init new resource manager")

	ResourceMgr.deviceInfo = devInfo

	Ranges := make(map[string]*openolt.DeviceInfo_DeviceResourceRanges)
	RsrcMgrsByTech := make(map[string]*ponrmgr.PONResourceManager)
	ResourceMgr.ResourceMgrs = make(map[uint32]*ponrmgr.PONResourceManager)

	// TODO self.args = registry('main').get_args()

	/*
	   If a legacy driver returns protobuf without any ranges,s synthesize one from
	   the legacy global per-device information. This, in theory, is temporary until
	   the legacy drivers are upgrade to support pool ranges.
	*/
	if devInfo.Ranges == nil {
		var ranges openolt.DeviceInfo_DeviceResourceRanges
		ranges.Technology = devInfo.GetTechnology()

		NumPONPorts := devInfo.GetPonPorts()
		var index uint32
		for index = 0; index < NumPONPorts; index++ {
			ranges.IntfIds = append(ranges.IntfIds, index)
		}

		var Pool openolt.DeviceInfo_DeviceResourceRanges_Pool
		Pool.Type = openolt.DeviceInfo_DeviceResourceRanges_Pool_ONU_ID
		Pool.Start = devInfo.OnuIdStart
		Pool.End = devInfo.OnuIdEnd
		Pool.Sharing = openolt.DeviceInfo_DeviceResourceRanges_Pool_DEDICATED_PER_INTF
		onuPool := Pool
		ranges.Pools = append(ranges.Pools, &onuPool)

		Pool.Type = openolt.DeviceInfo_DeviceResourceRanges_Pool_ALLOC_ID
		Pool.Start = devInfo.AllocIdStart
		Pool.End = devInfo.AllocIdEnd
		Pool.Sharing = openolt.DeviceInfo_DeviceResourceRanges_Pool_SHARED_BY_ALL_INTF_ALL_TECH
		allocPool := Pool
		ranges.Pools = append(ranges.Pools, &allocPool)

		Pool.Type = openolt.DeviceInfo_DeviceResourceRanges_Pool_GEMPORT_ID
		Pool.Start = devInfo.GemportIdStart
		Pool.End = devInfo.GemportIdEnd
		Pool.Sharing = openolt.DeviceInfo_DeviceResourceRanges_Pool_SHARED_BY_ALL_INTF_ALL_TECH
		gemPool := Pool
		ranges.Pools = append(ranges.Pools, &gemPool)

		Pool.Type = openolt.DeviceInfo_DeviceResourceRanges_Pool_FLOW_ID
		Pool.Start = devInfo.FlowIdStart
		Pool.End = devInfo.FlowIdEnd
		Pool.Sharing = openolt.DeviceInfo_DeviceResourceRanges_Pool_SHARED_BY_ALL_INTF_ALL_TECH
		ranges.Pools = append(ranges.Pools, &Pool)
		// Add to device info
		devInfo.Ranges = append(devInfo.Ranges, &ranges)
	}

	// Create a separate Resource Manager instance for each range. This assumes that
	// each technology is represented by only a single range
	var GlobalPONRsrcMgr *ponrmgr.PONResourceManager
	var err error
	IPPort := strings.Split(KVStoreHostPort, ":")
	for _, TechRange := range devInfo.Ranges {
		technology := TechRange.Technology
		log.Debugf("Device info technology %s", technology)
		Ranges[technology] = TechRange
		port, _ := strconv.Atoi(IPPort[1])
		RsrcMgrsByTech[technology], err = ponrmgr.NewPONResourceManager(technology, deviceType, deviceID,
			kvStoreType, IPPort[0], port)
		if err != nil {
			log.Errorf("Failed to create pon resource manager instance for technology %s", technology)
			return nil
		}
		// resource_mgrs_by_tech[technology] = resource_mgr
		if GlobalPONRsrcMgr == nil {
			GlobalPONRsrcMgr = RsrcMgrsByTech[technology]
		}
		for _, IntfID := range TechRange.IntfIds {
			ResourceMgr.ResourceMgrs[(IntfID)] = RsrcMgrsByTech[technology]
		}
		// self.initialize_device_resource_range_and_pool(resource_mgr, global_resource_mgr, arange)
		InitializeDeviceResourceRangeAndPool(RsrcMgrsByTech[technology], GlobalPONRsrcMgr,
			TechRange, devInfo)
	}
	// After we have initialized resource ranges, initialize the
	// resource pools accordingly.
	for _, PONRMgr := range RsrcMgrsByTech {
		_ = PONRMgr.InitDeviceResourcePool()
	}
	log.Info("Initialization of  resource manager success!")
	return &ResourceMgr
}

// InitializeDeviceResourceRangeAndPool initializes the resource range pool according to the sharing type, then apply
// device specific information. If KV doesn't exist
// or is broader than the device, the device's information will
// dictate the range limits
func InitializeDeviceResourceRangeAndPool(ponRMgr *ponrmgr.PONResourceManager, globalPONRMgr *ponrmgr.PONResourceManager,
	techRange *openolt.DeviceInfo_DeviceResourceRanges, devInfo *openolt.DeviceInfo) {

	// init the resource range pool according to the sharing type

	log.Debugf("Resource range pool init for technology %s", ponRMgr.Technology)
	// first load from KV profiles
	status := ponRMgr.InitResourceRangesFromKVStore()
	if !status {
		log.Debugf("Failed to load resource ranges from KV store for tech %s", ponRMgr.Technology)
	}

	/*
	   Then apply device specific information. If KV doesn't exist
	   or is broader than the device, the device's information will
	   dictate the range limits
	*/
	log.Debugf("Using device info to init pon resource ranges for tech", ponRMgr.Technology)

	ONUIDStart := devInfo.OnuIdStart
	ONUIDEnd := devInfo.OnuIdEnd
	ONUIDShared := openolt.DeviceInfo_DeviceResourceRanges_Pool_DEDICATED_PER_INTF
	ONUIDSharedPoolID := uint32(0)
	AllocIDStart := devInfo.AllocIdStart
	AllocIDEnd := devInfo.AllocIdEnd
	AllocIDShared := openolt.DeviceInfo_DeviceResourceRanges_Pool_SHARED_BY_ALL_INTF_ALL_TECH // TODO EdgeCore/BAL limitation
	AllocIDSharedPoolID := uint32(0)
	GEMPortIDStart := devInfo.GemportIdStart
	GEMPortIDEnd := devInfo.GemportIdEnd
	GEMPortIDShared := openolt.DeviceInfo_DeviceResourceRanges_Pool_SHARED_BY_ALL_INTF_ALL_TECH // TODO EdgeCore/BAL limitation
	GEMPortIDSharedPoolID := uint32(0)
	FlowIDStart := devInfo.FlowIdStart
	FlowIDEnd := devInfo.FlowIdEnd
	FlowIDShared := openolt.DeviceInfo_DeviceResourceRanges_Pool_SHARED_BY_ALL_INTF_ALL_TECH // TODO EdgeCore/BAL limitation
	FlowIDSharedPoolID := uint32(0)

	var FirstIntfPoolID uint32
	var SharedPoolID uint32

	/*
	 * As a zero check is made against SharedPoolID to check whether the resources are shared across all intfs
	 * if resources are shared across interfaces then SharedPoolID is given a positive number.
	 */
	for _, FirstIntfPoolID = range techRange.IntfIds {
		// skip the intf id 0
		if FirstIntfPoolID == 0 {
			continue
		}
		break
	}

	for _, RangePool := range techRange.Pools {
		if RangePool.Sharing == openolt.DeviceInfo_DeviceResourceRanges_Pool_SHARED_BY_ALL_INTF_ALL_TECH {
			SharedPoolID = FirstIntfPoolID
		} else if RangePool.Sharing == openolt.DeviceInfo_DeviceResourceRanges_Pool_SHARED_BY_ALL_INTF_SAME_TECH {
			SharedPoolID = FirstIntfPoolID
		} else {
			SharedPoolID = 0
		}
		if RangePool.Type == openolt.DeviceInfo_DeviceResourceRanges_Pool_ONU_ID {
			ONUIDStart = RangePool.Start
			ONUIDEnd = RangePool.End
			ONUIDShared = RangePool.Sharing
			ONUIDSharedPoolID = SharedPoolID
		} else if RangePool.Type == openolt.DeviceInfo_DeviceResourceRanges_Pool_ALLOC_ID {
			AllocIDStart = RangePool.Start
			AllocIDEnd = RangePool.End
			AllocIDShared = RangePool.Sharing
			AllocIDSharedPoolID = SharedPoolID
		} else if RangePool.Type == openolt.DeviceInfo_DeviceResourceRanges_Pool_GEMPORT_ID {
			GEMPortIDStart = RangePool.Start
			GEMPortIDEnd = RangePool.End
			GEMPortIDShared = RangePool.Sharing
			GEMPortIDSharedPoolID = SharedPoolID
		} else if RangePool.Type == openolt.DeviceInfo_DeviceResourceRanges_Pool_FLOW_ID {
			FlowIDStart = RangePool.Start
			FlowIDEnd = RangePool.End
			FlowIDShared = RangePool.Sharing
			FlowIDSharedPoolID = SharedPoolID
		}
	}

	log.Debugw("Device info init", log.Fields{"technology": techRange.Technology,
		"onu_id_start": ONUIDStart, "onu_id_end": ONUIDEnd, "onu_id_shared_pool_id": ONUIDSharedPoolID,
		"alloc_id_start": AllocIDStart, "alloc_id_end": AllocIDEnd,
		"alloc_id_shared_pool_id": AllocIDSharedPoolID,
		"gemport_id_start":        GEMPortIDStart, "gemport_id_end": GEMPortIDEnd,
		"gemport_id_shared_pool_id": GEMPortIDSharedPoolID,
		"flow_id_start":             FlowIDStart,
		"flow_id_end_idx":           FlowIDEnd,
		"flow_id_shared_pool_id":    FlowIDSharedPoolID,
		"intf_ids":                  techRange.IntfIds,
		"uni_id_start":              0,
		"uni_id_end_idx":            1, /*MaxUNIIDperONU()*/
	})

	ponRMgr.InitDefaultPONResourceRanges(ONUIDStart, ONUIDEnd, ONUIDSharedPoolID,
		AllocIDStart, AllocIDEnd, AllocIDSharedPoolID,
		GEMPortIDStart, GEMPortIDEnd, GEMPortIDSharedPoolID,
		FlowIDStart, FlowIDEnd, FlowIDSharedPoolID, 0, 1,
		devInfo.PonPorts, techRange.IntfIds)

	// For global sharing, make sure to refresh both local and global resource manager instances' range

	if ONUIDShared == openolt.DeviceInfo_DeviceResourceRanges_Pool_SHARED_BY_ALL_INTF_ALL_TECH {
		globalPONRMgr.UpdateRanges(ponrmgr.ONU_ID_START_IDX, ONUIDStart, ponrmgr.ONU_ID_END_IDX, ONUIDEnd,
			"", 0, nil)
		ponRMgr.UpdateRanges(ponrmgr.ONU_ID_START_IDX, ONUIDStart, ponrmgr.ONU_ID_END_IDX, ONUIDEnd,
			"", 0, globalPONRMgr)
	}
	if AllocIDShared == openolt.DeviceInfo_DeviceResourceRanges_Pool_SHARED_BY_ALL_INTF_ALL_TECH {
		globalPONRMgr.UpdateRanges(ponrmgr.ALLOC_ID_START_IDX, AllocIDStart, ponrmgr.ALLOC_ID_END_IDX, AllocIDEnd,
			"", 0, nil)

		ponRMgr.UpdateRanges(ponrmgr.ALLOC_ID_START_IDX, AllocIDStart, ponrmgr.ALLOC_ID_END_IDX, AllocIDEnd,
			"", 0, globalPONRMgr)
	}
	if GEMPortIDShared == openolt.DeviceInfo_DeviceResourceRanges_Pool_SHARED_BY_ALL_INTF_ALL_TECH {
		globalPONRMgr.UpdateRanges(ponrmgr.GEMPORT_ID_START_IDX, GEMPortIDStart, ponrmgr.GEMPORT_ID_END_IDX, GEMPortIDEnd,
			"", 0, nil)
		ponRMgr.UpdateRanges(ponrmgr.GEMPORT_ID_START_IDX, GEMPortIDStart, ponrmgr.GEMPORT_ID_END_IDX, GEMPortIDEnd,
			"", 0, globalPONRMgr)
	}
	if FlowIDShared == openolt.DeviceInfo_DeviceResourceRanges_Pool_SHARED_BY_ALL_INTF_ALL_TECH {
		globalPONRMgr.UpdateRanges(ponrmgr.FLOW_ID_START_IDX, FlowIDStart, ponrmgr.FLOW_ID_END_IDX, FlowIDEnd,
			"", 0, nil)
		ponRMgr.UpdateRanges(ponrmgr.FLOW_ID_START_IDX, FlowIDStart, ponrmgr.FLOW_ID_END_IDX, FlowIDEnd,
			"", 0, globalPONRMgr)
	}

	// Make sure loaded range fits the platform bit encoding ranges
	ponRMgr.UpdateRanges(ponrmgr.UNI_ID_START_IDX, 0, ponrmgr.UNI_ID_END_IDX /* TODO =OpenOltPlatform.MAX_UNIS_PER_ONU-1*/, 1, "", 0, nil)
}

// Delete clears used resources for the particular olt device being deleted
func (RsrcMgr *OpenOltResourceMgr) Delete() error {
	/* TODO
	   def __del__(self):
	           self.log.info("clearing-device-resource-pool")
	           for key, resource_mgr in self.resource_mgrs.iteritems():
	               resource_mgr.clear_device_resource_pool()

	       def assert_pon_id_limit(self, pon_intf_id):
	           assert pon_intf_id in self.resource_mgrs

	       def assert_onu_id_limit(self, pon_intf_id, onu_id):
	           self.assert_pon_id_limit(pon_intf_id)
	           self.resource_mgrs[pon_intf_id].assert_resource_limits(onu_id, PONResourceManager.ONU_ID)

	       @property
	       def max_uni_id_per_onu(self):
	           return 0 #OpenOltPlatform.MAX_UNIS_PER_ONU-1, zero-based indexing Uncomment or override to make default multi-uni

	       def assert_uni_id_limit(self, pon_intf_id, onu_id, uni_id):
	           self.assert_onu_id_limit(pon_intf_id, onu_id)
	           self.resource_mgrs[pon_intf_id].assert_resource_limits(uni_id, PONResourceManager.UNI_ID)
	*/
	for _, rsrcMgr := range RsrcMgr.ResourceMgrs {
		if err := rsrcMgr.ClearDeviceResourcePool(); err != nil {
			log.Debug("Failed to clear device resource pool")
			return err
		}
	}
	log.Debug("Cleared device resource pool")
	return nil
}

// GetONUID returns the available OnuID for the given pon-port
func (RsrcMgr *OpenOltResourceMgr) GetONUID(ponIntfID uint32) (uint32, error) {
	// Check if Pon Interface ID is present in Resource-manager-map
	if _, ok := RsrcMgr.ResourceMgrs[ponIntfID]; !ok {
		err := errors.New("invalid-pon-interface-" + strconv.Itoa(int(ponIntfID)))
		return 0, err
	}
	// Get ONU id for a provided pon interface ID.
	ONUID, err := RsrcMgr.ResourceMgrs[ponIntfID].GetResourceID(ponIntfID,
		ponrmgr.ONU_ID, 1)
	if err != nil {
		log.Errorf("Failed to get resource for interface %d for type %s",
			ponIntfID, ponrmgr.ONU_ID)
		return 0, err
	}
	if ONUID != nil {
		RsrcMgr.ResourceMgrs[ponIntfID].InitResourceMap(fmt.Sprintf("%d,%d", ponIntfID, ONUID[0]))
		return ONUID[0], err
	}

	return 0, err // return OnuID 0 on error
}

// GetAllocID return the first Alloc ID for a given pon interface id and onu id and then update the resource map on
// the KV store with the list of alloc_ids allocated for the pon_intf_onu_id tuple
// Currently of all the alloc_ids available, it returns the first alloc_id in the list for tha given ONU
func (RsrcMgr *OpenOltResourceMgr) GetAllocID(intfID uint32, onuID uint32, uniID uint32) uint32 {

	var err error
	IntfOnuIDUniID := fmt.Sprintf("%d,%d,%d", intfID, onuID, uniID)
	AllocID := RsrcMgr.ResourceMgrs[intfID].GetCurrentAllocIDForOnu(IntfOnuIDUniID)
	if AllocID != nil {
		// Since we support only one alloc_id for the ONU at the moment,
		// return the first alloc_id in the list, if available, for that
		// ONU.
		log.Debugw("Retrieved alloc ID from pon resource mgr", log.Fields{"AllocID": AllocID})
		return AllocID[0]
	}
	AllocID, err = RsrcMgr.ResourceMgrs[intfID].GetResourceID(intfID,
		ponrmgr.ALLOC_ID, 1)

	if AllocID == nil || err != nil {
		log.Error("Failed to allocate alloc id")
		return 0
	}
	// update the resource map on KV store with the list of alloc_id
	// allocated for the pon_intf_onu_id tuple
	err = RsrcMgr.ResourceMgrs[intfID].UpdateAllocIdsForOnu(IntfOnuIDUniID, AllocID)
	if err != nil {
		log.Error("Failed to update Alloc ID")
		return 0
	}
	log.Debugw("Allocated new Tcont from pon resource mgr", log.Fields{"AllocID": AllocID})
	return AllocID[0]
}

// GetGEMPortID gets gem port id for a particular pon port, onu id and uni id and then update the resource map on
// the KV store with the list of gemport_id allocated for the pon_intf_onu_id tuple
func (RsrcMgr *OpenOltResourceMgr) GetGEMPortID(ponPort uint32, onuID uint32,
	uniID uint32, NumOfPorts uint32) ([]uint32, error) {

	/* Get gem port id for a particular pon port, onu id
	   and uni id.
	*/

	var err error
	IntfOnuIDUniID := fmt.Sprintf("%d,%d,%d", ponPort, onuID, uniID)

	GEMPortList := RsrcMgr.ResourceMgrs[ponPort].GetCurrentGEMPortIDsForOnu(IntfOnuIDUniID)
	if GEMPortList != nil {
		return GEMPortList, nil
	}

	GEMPortList, err = RsrcMgr.ResourceMgrs[ponPort].GetResourceID(ponPort,
		ponrmgr.GEMPORT_ID, NumOfPorts)
	if err != nil && GEMPortList == nil {
		log.Errorf("Failed to get gem port id for %s", IntfOnuIDUniID)
		return nil, err
	}

	// update the resource map on KV store with the list of gemport_id
	// allocated for the pon_intf_onu_id tuple
	err = RsrcMgr.ResourceMgrs[ponPort].UpdateGEMPortIDsForOnu(IntfOnuIDUniID,
		GEMPortList)
	if err != nil {
		log.Errorf("Failed to update GEM ports to kv store for %s", IntfOnuIDUniID)
		return nil, err
	}

	return GEMPortList, err
}

// FreeFlowID returns the free flow id for a given interface, onu id and uni id
func (RsrcMgr *OpenOltResourceMgr) FreeFlowID(IntfID uint32, onuID int32,
	uniID int32, FlowID uint32) {
	var IntfONUID string
	var err error
	FlowIds := make([]uint32, 0)

	FlowIds = append(FlowIds, FlowID)
	IntfONUID = fmt.Sprintf("%d,%d,%d", IntfID, onuID, uniID)
	err = RsrcMgr.ResourceMgrs[IntfID].UpdateFlowIDForOnu(IntfONUID, FlowID, false)
	if err != nil {
		log.Errorw("Failed to Update flow id  for", log.Fields{"intf": IntfONUID})
	}
	RsrcMgr.ResourceMgrs[IntfID].RemoveFlowIDInfo(IntfONUID, FlowID)
	RsrcMgr.ResourceMgrs[IntfID].FreeResourceID(IntfID, ponrmgr.FLOW_ID, FlowIds)
}

// FreeFlowIDs releases the flow Ids
func (RsrcMgr *OpenOltResourceMgr) FreeFlowIDs(IntfID uint32, onuID uint32,
	uniID uint32, FlowID []uint32) {

	RsrcMgr.ResourceMgrs[IntfID].FreeResourceID(IntfID, ponrmgr.FLOW_ID, FlowID)

	var IntfOnuIDUniID string
	var err error
	for _, flow := range FlowID {
		IntfOnuIDUniID = fmt.Sprintf("%d,%d,%d", IntfID, onuID, uniID)
		err = RsrcMgr.ResourceMgrs[IntfID].UpdateFlowIDForOnu(IntfOnuIDUniID, flow, false)
		if err != nil {
			log.Errorw("Failed to Update flow id for", log.Fields{"intf": IntfOnuIDUniID})
		}
		RsrcMgr.ResourceMgrs[IntfID].RemoveFlowIDInfo(IntfOnuIDUniID, flow)
	}
}

// FreeAllocID frees AllocID on the PON resource pool and also frees the allocID association
// for the given OLT device.
func (RsrcMgr *OpenOltResourceMgr) FreeAllocID(IntfID uint32, allocID uint32) {
	allocIDs := make([]uint32, 0)
	allocIDs = append(allocIDs, allocID)
	RsrcMgr.ResourceMgrs[IntfID].FreeResourceID(IntfID, ponrmgr.ALLOC_ID, allocIDs)
}

// FreeGemPortID frees GemPortID on the PON resource pool and also frees the gemPortID association
// for the given OLT device.
func (RsrcMgr *OpenOltResourceMgr) FreeGemPortID(IntfID uint32, gemPortID uint32) {
	gemPortIDs := make([]uint32, 0)
	gemPortIDs = append(gemPortIDs, gemPortID)
	RsrcMgr.ResourceMgrs[IntfID].FreeResourceID(IntfID, ponrmgr.GEMPORT_ID, gemPortIDs)
}
