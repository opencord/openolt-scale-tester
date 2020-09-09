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
	"sync"

	"github.com/opencord/voltha-lib-go/v4/pkg/log"
	ponrmgr "github.com/opencord/voltha-lib-go/v4/pkg/ponresourcemanager"
	"github.com/opencord/voltha-protos/v4/go/openolt"
	"golang.org/x/net/context"
)

// OpenOltResourceMgr holds resource related information as provided below for each field
type OpenOltResourceMgr struct {
	deviceInfo *openolt.DeviceInfo

	// This protects concurrent onu_id allocate/delete calls on a per PON port basis
	OnuIDMgmtLock []sync.RWMutex
	// This protects concurrent flow_id allocate/delete calls. We do not need this on a
	// per PON port basis as flow IDs are unique across the OLT.
	FlowIDMgmtLock sync.RWMutex

	// This protects concurrent GemID and AllocID allocate/delete calls on a per PON port basis
	GemIDAllocIDLock []sync.RWMutex

	// array of pon resource managers per interface technology
	ResourceMgrs map[uint32]*ponrmgr.PONResourceManager

	flow_id uint64
}

// NewResourceMgr init a New resource manager instance which in turn instantiates pon resource manager
// instances according to technology. Initializes the default resource ranges for all
// the resources.
func NewResourceMgr(deviceID string, KVStoreHostPort string, kvStoreType string, deviceType string, devInfo *openolt.DeviceInfo) *OpenOltResourceMgr {
	var ResourceMgr OpenOltResourceMgr
	logger.Debugf(nil, "Init new resource manager")

	ResourceMgr.deviceInfo = devInfo
	NumPONPorts := devInfo.GetPonPorts()

	ResourceMgr.OnuIDMgmtLock = make([]sync.RWMutex, NumPONPorts)
	ResourceMgr.GemIDAllocIDLock = make([]sync.RWMutex, NumPONPorts)
	ResourceMgr.FlowIDMgmtLock = sync.RWMutex{}

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
	for _, TechRange := range devInfo.Ranges {
		technology := TechRange.Technology
		logger.Debugf(nil, "Device info technology %s", technology)
		Ranges[technology] = TechRange
		RsrcMgrsByTech[technology], err = ponrmgr.NewPONResourceManager(nil, technology, deviceType, deviceID,
			kvStoreType, KVStoreHostPort)
		if err != nil {
			logger.Errorf(nil, "Failed to create pon resource manager instance for technology %s", technology)
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
		_ = PONRMgr.InitDeviceResourcePool(context.Background())
	}
	logger.Info(nil, "Initialization of  resource manager success!")
	return &ResourceMgr
}

// InitializeDeviceResourceRangeAndPool initializes the resource range pool according to the sharing type, then apply
// device specific information. If KV doesn't exist
// or is broader than the device, the device's information will
// dictate the range limits
func InitializeDeviceResourceRangeAndPool(ponRMgr *ponrmgr.PONResourceManager, globalPONRMgr *ponrmgr.PONResourceManager,
	techRange *openolt.DeviceInfo_DeviceResourceRanges, devInfo *openolt.DeviceInfo) {

	// init the resource range pool according to the sharing type

	logger.Debugf(nil, "Resource range pool init for technology %s", ponRMgr.Technology)
	// first load from KV profiles
	status := ponRMgr.InitResourceRangesFromKVStore(context.Background())
	if !status {
		logger.Debugf(nil, "Failed to load resource ranges from KV store for tech %s", ponRMgr.Technology)
	}

	/*
	   Then apply device specific information. If KV doesn't exist
	   or is broader than the device, the device's information will
	   dictate the range limits
	*/
	logger.Debugf(nil, "Using device info to init pon resource ranges for tech", ponRMgr.Technology)

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

	logger.Debugw(nil, "Device info init", log.Fields{"technology": techRange.Technology,
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

	ponRMgr.InitDefaultPONResourceRanges(nil, ONUIDStart, ONUIDEnd, ONUIDSharedPoolID,
		AllocIDStart, AllocIDEnd, AllocIDSharedPoolID,
		GEMPortIDStart, GEMPortIDEnd, GEMPortIDSharedPoolID,
		FlowIDStart, FlowIDEnd, FlowIDSharedPoolID, 0, 1,
		devInfo.PonPorts, techRange.IntfIds)

	// For global sharing, make sure to refresh both local and global resource manager instances' range

	if ONUIDShared == openolt.DeviceInfo_DeviceResourceRanges_Pool_SHARED_BY_ALL_INTF_ALL_TECH {
		globalPONRMgr.UpdateRanges(nil, ponrmgr.ONU_ID_START_IDX, ONUIDStart, ponrmgr.ONU_ID_END_IDX, ONUIDEnd,
			"", 0, nil)
		ponRMgr.UpdateRanges(nil, ponrmgr.ONU_ID_START_IDX, ONUIDStart, ponrmgr.ONU_ID_END_IDX, ONUIDEnd,
			"", 0, globalPONRMgr)
	}
	if AllocIDShared == openolt.DeviceInfo_DeviceResourceRanges_Pool_SHARED_BY_ALL_INTF_ALL_TECH {
		globalPONRMgr.UpdateRanges(nil, ponrmgr.ALLOC_ID_START_IDX, AllocIDStart, ponrmgr.ALLOC_ID_END_IDX, AllocIDEnd,
			"", 0, nil)

		ponRMgr.UpdateRanges(nil, ponrmgr.ALLOC_ID_START_IDX, AllocIDStart, ponrmgr.ALLOC_ID_END_IDX, AllocIDEnd,
			"", 0, globalPONRMgr)
	}
	if GEMPortIDShared == openolt.DeviceInfo_DeviceResourceRanges_Pool_SHARED_BY_ALL_INTF_ALL_TECH {
		globalPONRMgr.UpdateRanges(nil, ponrmgr.GEMPORT_ID_START_IDX, GEMPortIDStart, ponrmgr.GEMPORT_ID_END_IDX, GEMPortIDEnd,
			"", 0, nil)
		ponRMgr.UpdateRanges(nil, ponrmgr.GEMPORT_ID_START_IDX, GEMPortIDStart, ponrmgr.GEMPORT_ID_END_IDX, GEMPortIDEnd,
			"", 0, globalPONRMgr)
	}
	if FlowIDShared == openolt.DeviceInfo_DeviceResourceRanges_Pool_SHARED_BY_ALL_INTF_ALL_TECH {
		globalPONRMgr.UpdateRanges(nil, ponrmgr.FLOW_ID_START_IDX, FlowIDStart, ponrmgr.FLOW_ID_END_IDX, FlowIDEnd,
			"", 0, nil)
		ponRMgr.UpdateRanges(nil, ponrmgr.FLOW_ID_START_IDX, FlowIDStart, ponrmgr.FLOW_ID_END_IDX, FlowIDEnd,
			"", 0, globalPONRMgr)
	}

	// Make sure loaded range fits the platform bit encoding ranges
	ponRMgr.UpdateRanges(nil, ponrmgr.UNI_ID_START_IDX, 0, ponrmgr.UNI_ID_END_IDX /* TODO =OpenOltPlatform.MAX_UNIS_PER_ONU-1*/, 1, "", 0, nil)
}

// Delete clears used resources for the particular olt device being deleted
func (RsrcMgr *OpenOltResourceMgr) Delete() error {
	for _, rsrcMgr := range RsrcMgr.ResourceMgrs {
		if err := rsrcMgr.ClearDeviceResourcePool(context.Background()); err != nil {
			logger.Debug(nil, "Failed to clear device resource pool")
			return err
		}
	}
	logger.Debug(nil, "Cleared device resource pool")
	return nil
}

// GetONUID returns the available OnuID for the given pon-port
func (RsrcMgr *OpenOltResourceMgr) GetONUID(ponIntfID uint32) (uint32, error) {
	RsrcMgr.OnuIDMgmtLock[ponIntfID].Lock()
	defer RsrcMgr.OnuIDMgmtLock[ponIntfID].Unlock()
	// Check if Pon Interface ID is present in Resource-manager-map
	ONUIDs, err := RsrcMgr.ResourceMgrs[ponIntfID].GetResourceID(context.Background(), ponIntfID,
		ponrmgr.ONU_ID, 1)
	if err != nil {
		logger.Errorf(nil, "Failed to get resource for interface %d for type %s",
			ponIntfID, ponrmgr.ONU_ID)
		return uint32(0), err
	}
	return ONUIDs[0], err
}

// GetFlowID return flow ID for a given pon interface id, onu id and uni id
func (RsrcMgr *OpenOltResourceMgr) GetFlowID(ctx context.Context, ponIntfID uint32) (uint64, error) {
	RsrcMgr.FlowIDMgmtLock.Lock()
	defer RsrcMgr.FlowIDMgmtLock.Unlock()
	RsrcMgr.flow_id++
	return RsrcMgr.flow_id, nil
}
