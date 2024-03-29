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
	"sync"

	"github.com/opencord/voltha-lib-go/v7/pkg/log"
	"github.com/opencord/voltha-protos/v5/go/openolt"
)

type DtStagKey struct {
	ponIntf, onuID, uniID uint32
}

var currDtStag uint32
var DtStag map[DtStagKey]uint32
var DtCtag map[uint32]uint32
var AttCtag map[uint32]uint32
var TtCtag map[uint32]uint32
var mutex sync.RWMutex

func init() {
	AttCtag = make(map[uint32]uint32)
	DtCtag = make(map[uint32]uint32)
	DtStag = make(map[DtStagKey]uint32)
	TtCtag = make(map[uint32]uint32)
}

const (
	vendorName = "ABCD"
	// Number of bits for the physical UNI of the ONUs
	bitsForUniID = 4
	// Number of bits for the ONU ID
	bitsForONUID = 8
	//MaxOnusPerPon is Max number of ONUs on any PON port
	MaxOnusPerPon = 1 << bitsForONUID
)

var vendorSpecificId = 1000

func GenerateNextONUSerialNumber() *openolt.SerialNumber {

	vi := []byte(vendorName)

	vendorSpecificId += 1
	vs := []byte(fmt.Sprint(vendorSpecificId))
	// logger.Infow(nil, "vendor-id-and-vendor-specific", log.Fields{"vi":vi, "vs":vs})
	sn := &openolt.SerialNumber{VendorId: vi, VendorSpecific: vs}
	// logger.Infow(nil, "serial-num", log.Fields{"sn":sn})

	return sn
}

//MkUniPortNum returns new UNIportNum based on intfID, inuID and uniID
func MkUniPortNum(intfID, onuID, uniID uint32) uint32 {
	var limit = int(onuID)
	if limit > MaxOnusPerPon {
		logger.Warn(nil, "Warning: exceeded the MAX ONUS per PON")
	}
	return (intfID << (bitsForUniID + bitsForONUID)) | (onuID << bitsForUniID) | uniID
}

func GetAttCtag(ponIntf uint32) uint32 {
	var currCtag uint32
	var ok bool
	mutex.Lock()
	defer mutex.Unlock()
	if currCtag, ok = AttCtag[ponIntf]; !ok {
		// Start with ctag 2
		AttCtag[ponIntf] = 2
		return AttCtag[ponIntf]
	}
	AttCtag[ponIntf] = currCtag + 1
	return AttCtag[ponIntf]
}

func GetDtCtag(ponIntf uint32) uint32 {
	var currCtag uint32
	var ok bool
	mutex.Lock()
	defer mutex.Unlock()
	if currCtag, ok = DtCtag[ponIntf]; !ok {
		// Start with ctag 1
		DtCtag[ponIntf] = 1
		return DtCtag[ponIntf]
	}
	DtCtag[ponIntf] = currCtag + 1
	return DtCtag[ponIntf]
}

func GetTtCtag(ponIntf uint32) uint32 {
	var currCtag uint32
	var ok bool
	mutex.Lock()
	defer mutex.Unlock()
	if currCtag, ok = TtCtag[ponIntf]; !ok {
		// Start with ctag 1
		TtCtag[ponIntf] = 1
		return TtCtag[ponIntf]
	}
	TtCtag[ponIntf] = currCtag + 1
	return TtCtag[ponIntf]
}

func GetAttStag(ponIntf uint32) uint32 {
	// start with stag 2
	return ponIntf + 2
}

func GetDtStag(ponIntf uint32, onuID uint32, uniID uint32) uint32 {
	// Dt workflow requires unique stag for each subscriber
	key := DtStagKey{ponIntf: ponIntf, onuID: onuID, uniID: uniID}
	mutex.Lock()
	defer mutex.Unlock()
	if value, ok := DtStag[key]; ok {
		return value
	} else {
		DtStag[key] = currDtStag + 1
		currDtStag = DtStag[key]
	}
	return DtStag[key]
}

func GetTtStag(ponIntf uint32) uint32 {
	// start with stag 2
	return ponIntf + 2
}

// TODO: More workflow support to be added here
func GetCtag(workFlowName string, ponIntf uint32) uint32 {
	switch workFlowName {
	case "ATT":
		return GetAttCtag(ponIntf)
	case "DT":
		return GetDtCtag(ponIntf)
	case "TT":
		return GetTtCtag(ponIntf)
	default:
		logger.Errorw(nil, "unknown-workflowname", log.Fields{"workflow": workFlowName})
	}
	return 0
}

func GetStag(workFlowName string, ponIntf uint32, onuID uint32, uniID uint32) uint32 {
	switch workFlowName {
	case "ATT":
		return GetAttStag(ponIntf)
	case "DT":
		return GetDtStag(ponIntf, onuID, uniID)
	case "TT":
		return GetTtStag(ponIntf)
	default:
		logger.Errorw(nil, "unknown-workflowname", log.Fields{"workflow": workFlowName})
	}
	return 0
}
