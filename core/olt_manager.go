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
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/cenkalti/backoff/v3"
	"github.com/opencord/openolt-scale-tester/config"
	"github.com/opencord/voltha-lib-go/v4/pkg/db/kvstore"
	"github.com/opencord/voltha-lib-go/v4/pkg/log"
	"github.com/opencord/voltha-lib-go/v4/pkg/techprofile"
	oop "github.com/opencord/voltha-protos/v4/go/openolt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	ReasonOk          = "OK"
	TechProfileKVPath = "service/voltha/technology_profiles/%s/%d" // service/voltha/technology_profiles/xgspon/<tech_profile_tableID>
)

type OnuDeviceKey struct {
	onuID    uint32
	ponInfID uint32
}

type OpenOltManager struct {
	ipPort             string
	deviceInfo         *oop.DeviceInfo
	OnuDeviceMap       map[OnuDeviceKey]*OnuDevice `json:"onuDeviceMap"`
	TechProfile        map[uint32]*techprofile.TechProfileIf
	clientConn         *grpc.ClientConn
	openOltClient      oop.OpenoltClient
	testConfig         *config.OpenOltScaleTesterConfig
	rsrMgr             *OpenOltResourceMgr
	lockRsrAlloc       sync.RWMutex
	lockOpenOltManager sync.RWMutex
}

func NewOpenOltManager(ipPort string) *OpenOltManager {
	logger.Infow(nil, "initialized openolt manager with ipPort", log.Fields{"ipPort": ipPort})
	return &OpenOltManager{
		ipPort:             ipPort,
		OnuDeviceMap:       make(map[OnuDeviceKey]*OnuDevice),
		lockRsrAlloc:       sync.RWMutex{},
		lockOpenOltManager: sync.RWMutex{},
	}
}

func (om *OpenOltManager) readAndLoadTPsToEtcd() {
	var byteValue []byte
	var err error
	// Verify that etcd is up before starting the application.
	etcdIpPort := "http://" + om.testConfig.KVStoreHost + ":" + strconv.Itoa(om.testConfig.KVStorePort)
	client, err := kvstore.NewEtcdClient(context.Background(), etcdIpPort, 5*time.Second, log.FatalLevel)
	if err != nil || client == nil {
		logger.Fatal(nil, "error-initializing-etcd-client")
		return
	}

	// Load TPs to etcd for each of the specified tech-profiles
	for _, tpID := range om.testConfig.TpIDList {
		// Below should translate to something like "/app/ATT-64.json"
		// The TP file should exist.
		tpFilePath := "/app/" + om.testConfig.WorkflowName + "-" + strconv.Itoa(tpID) + ".json"
		// Open our jsonFile
		jsonFile, err := os.Open(tpFilePath)
		// if we os.Open returns an error then handle it
		if err != nil {
			logger.Fatalw(nil, "could-not-find-tech-profile", log.Fields{"err": err, "tpFile": tpFilePath})
		}
		logger.Debugw(nil, "tp-file-opened-successfully", log.Fields{"tpFile": tpFilePath})

		// read our opened json file as a byte array.
		if byteValue, err = ioutil.ReadAll(jsonFile); err != nil {
			logger.Fatalw(nil, "could-not-read-tp-file", log.Fields{"err": err, "tpFile": tpFilePath})
		}

		var tp techprofile.TechProfile

		if err = json.Unmarshal(byteValue, &tp); err != nil {
			logger.Fatalw(nil, "could-not-unmarshal-tp", log.Fields{"err": err, "tpFile": tpFilePath})
		} else {
			logger.Infow(nil, "tp-read-from-file", log.Fields{"tp": tp, "tpFile": tpFilePath})
		}
		kvPath := fmt.Sprintf(TechProfileKVPath, om.deviceInfo.Technology, tpID)
		tpJson, err := json.Marshal(tp)
		err = client.Put(context.Background(), kvPath, tpJson)
		if err != nil {
			logger.Fatalw(nil, "tp-put-to-etcd-failed", log.Fields{"tpPath": kvPath, "err": err})
		}
		// verify the PUT succeeded.
		kvResult, err := client.Get(context.Background(), kvPath)
		if kvResult == nil {
			logger.Fatal(nil, "tp-not-found-on-kv-after-load", log.Fields{"key": kvPath, "err": err})
		} else {
			var KvTpIns techprofile.TechProfile
			var resPtr = &KvTpIns
			if value, err := kvstore.ToByte(kvResult.Value); err == nil {
				if err = json.Unmarshal(value, resPtr); err != nil {
					logger.Fatal(nil, "error-unmarshal-kv-result", log.Fields{"err": err, "key": kvPath, "value": value})
				} else {
					logger.Infow(nil, "verified-ok-that-tp-load-was-good", log.Fields{"tpID": tpID, "kvPath": kvPath})
					_ = jsonFile.Close()
					continue
				}
			}
		}
	}
}

func (om *OpenOltManager) Start(testConfig *config.OpenOltScaleTesterConfig) error {
	var err error
	om.testConfig = testConfig

	// Establish gRPC connection with the device
	if om.clientConn, err = grpc.Dial(om.ipPort, grpc.WithInsecure(), grpc.WithBlock()); err != nil {
		logger.Errorw(nil, "Failed to dial device", log.Fields{"ipPort": om.ipPort, "err": err})
		return err
	}
	om.openOltClient = oop.NewOpenoltClient(om.clientConn)

	// Populate Device Info
	if deviceInfo, err := om.populateDeviceInfo(); err != nil {
		logger.Error(nil, "error fetching device info", log.Fields{"err": err, "deviceInfo": deviceInfo})
		return err
	}

	// Read and load TPs to etcd.
	om.readAndLoadTPsToEtcd()

	logger.Info(nil, "etcd-up-and-running--tp-loaded-successfully")

	if om.rsrMgr = NewResourceMgr("ABCD", om.testConfig.KVStoreHost+":"+strconv.Itoa(om.testConfig.KVStorePort),
		"etcd", "openolt", om.deviceInfo); om.rsrMgr == nil {
		logger.Error(nil, "Error while instantiating resource manager")
		return errors.New("instantiating resource manager failed")
	}

	om.TechProfile = make(map[uint32]*techprofile.TechProfileIf)
	if err = om.populateTechProfilePerPonPort(); err != nil {
		logger.Error(nil, "Error while populating tech profile mgr\n")
		return errors.New("error-loading-tech-profile-per-ponPort")
	}

	// Start reading indications
	go om.readIndications()

	// Provision OLT NNI Trap flows as needed by the Workflow
	if err = ProvisionNniTrapFlow(om.openOltClient, om.testConfig, om.rsrMgr); err != nil {
		logger.Error(nil, "failed-to-add-nni-trap-flow", log.Fields{"err": err})
	}

	// Provision ONUs one by one
	go om.provisionONUs()

	return nil

}

func (om *OpenOltManager) populateDeviceInfo() (*oop.DeviceInfo, error) {
	var err error

	if om.deviceInfo, err = om.openOltClient.GetDeviceInfo(context.Background(), new(oop.Empty)); err != nil {
		logger.Errorw(nil, "Failed to fetch device info", log.Fields{"err": err})
		return nil, err
	}

	if om.deviceInfo == nil {
		logger.Errorw(nil, "Device info is nil", log.Fields{})
		return nil, errors.New("failed to get device info from OLT")
	}

	logger.Debugw(nil, "Fetched device info", log.Fields{"deviceInfo": om.deviceInfo})

	return om.deviceInfo, nil
}

func (om *OpenOltManager) provisionONUs() {
	var numOfONUsPerPon uint
	var i, j, k, onuID uint32
	var err error
	var onuWg sync.WaitGroup

	defer func() {
		// Stop the process once the job is done
		_ = syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	}()

	// If the number of ONUs to provision is not a power of 2, stop execution
	// This is needed for ensure even distribution of ONUs across all PONs
	if !isPowerOfTwo(om.testConfig.NumOfOnu) {
		logger.Errorw(nil, "num-of-onus-to-provision-is-not-a-power-of-2", log.Fields{"numOfOnus": om.testConfig.NumOfOnu})
		return
	}

	// Number of ONUs to provision should not be less than the number of PON ports.
	// We need at least one ONU per PON
	if om.testConfig.NumOfOnu < uint(om.deviceInfo.PonPorts) {
		logger.Errorw(nil, "num-of-onu-is-less-than-num-of-pon-port", log.Fields{"numOfOnus": om.testConfig.NumOfOnu, "numOfPon": om.deviceInfo.PonPorts})
		return
	}

	numOfONUsPerPon = om.testConfig.NumOfOnu / uint(om.deviceInfo.PonPorts)
	totalOnusToProvision := numOfONUsPerPon * uint(om.deviceInfo.PonPorts)
	logger.Infow(nil, "***** all-onu-provision-started ******",
		log.Fields{"totalNumOnus": totalOnusToProvision,
			"numOfOnusPerPon": numOfONUsPerPon,
			"numOfPons":       om.deviceInfo.PonPorts})

	// These are the number of ONUs that will be provisioned per PON port per batch.
	// Such number of ONUs will be chosen across all PON ports per batch
	var onusPerIterationPerPonPort uint32 = 4

	// If the total number of ONUs per PON is lesser than the default ONU to provision per pon port per batch
	// then keep halving the ONU to provision per pon port per batch until we reach an acceptable number
	// Note: the least possible value for onusPerIterationPerPonPort is 1
	for uint32(numOfONUsPerPon) < onusPerIterationPerPonPort {
		onusPerIterationPerPonPort /= 2
	}

	startTime := time.Now()
	// Start provisioning the ONUs
	for i = 0; i < uint32(numOfONUsPerPon)/onusPerIterationPerPonPort; i++ {
		for j = 0; j < om.deviceInfo.PonPorts; j++ {
			for k = 0; k < onusPerIterationPerPonPort; k++ {
				om.lockRsrAlloc.Lock()
				sn := GenerateNextONUSerialNumber()
				om.lockRsrAlloc.Unlock()
				logger.Debugw(nil, "provisioning onu", log.Fields{"onuID": j, "ponPort": i, "serialNum": sn})
				if onuID, err = om.rsrMgr.GetONUID(j); err != nil {
					logger.Errorw(nil, "error getting onu id", log.Fields{"err": err})
					continue
				}
				logger.Infow(nil, "onu-provision-started-from-olt-manager", log.Fields{"onuId": onuID, "ponIntf": i})

				onuWg.Add(1)
				go om.activateONU(j, onuID, sn, om.stringifySerialNumber(sn), &onuWg)
			}
		}
		// Wait for the group of ONUs to complete processing before going to next batch of ONUs
		onuWg.Wait()
	}
	endTime := time.Now()
	logger.Info(nil, "******** all-onu-provisioning-completed *******")
	totalTime := endTime.Sub(startTime)
	out := time.Time{}.Add(totalTime)
	logger.Infof(nil, "****** Total Time to provision all the ONUs is => %s", out.Format("15:04:05"))

	// TODO: We need to dump the results at the end. But below json marshall does not work. We will need custom Marshal function.
	/*
		e, err := json.Marshal(om)
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(string(e))
	*/
}

func (om *OpenOltManager) activateONU(intfID uint32, onuID uint32, serialNum *oop.SerialNumber, serialNumber string, onuWg *sync.WaitGroup) {
	logger.Debugw(nil, "activate-onu", log.Fields{"intfID": intfID, "onuID": onuID, "serialNum": serialNum, "serialNumber": serialNumber})
	// TODO: need resource manager
	var pir uint32 = 1000000
	var onuDevice = OnuDevice{
		SerialNum:     serialNumber,
		OnuID:         onuID,
		PonIntf:       intfID,
		openOltClient: om.openOltClient,
		testConfig:    om.testConfig,
		rsrMgr:        om.rsrMgr,
		onuWg:         onuWg,
	}
	var err error
	onuDeviceKey := OnuDeviceKey{onuID: onuID, ponInfID: intfID}
	Onu := oop.Onu{IntfId: intfID, OnuId: onuID, SerialNumber: serialNum, Pir: pir}
	now := time.Now()
	nanos := now.UnixNano()
	milliStart := nanos / 1000000
	onuDevice.OnuProvisionStartTime = time.Unix(0, nanos)
	if _, err = om.openOltClient.ActivateOnu(context.Background(), &Onu); err != nil {
		st, _ := status.FromError(err)
		if st.Code() == codes.AlreadyExists {
			logger.Debug(nil, "ONU activation is in progress", log.Fields{"SerialNumber": serialNumber})
		} else {
			nanos = now.UnixNano()
			milliEnd := nanos / 1000000
			onuDevice.OnuProvisionEndTime = time.Unix(0, nanos)
			onuDevice.OnuProvisionDurationInMs = milliEnd - milliStart
			logger.Errorw(nil, "activate-onu-failed", log.Fields{"Onu": Onu, "err ": err})
			onuDevice.Reason = err.Error()
		}
	} else {
		nanos = now.UnixNano()
		milliEnd := nanos / 1000000
		onuDevice.OnuProvisionEndTime = time.Unix(0, nanos)
		onuDevice.OnuProvisionDurationInMs = milliEnd - milliStart
		onuDevice.Reason = ReasonOk
		logger.Infow(nil, "activated-onu", log.Fields{"SerialNumber": serialNumber})
	}

	om.lockOpenOltManager.Lock()
	om.OnuDeviceMap[onuDeviceKey] = &onuDevice
	om.lockOpenOltManager.Unlock()

	// If ONU activation was success provision the ONU
	if err == nil {
		om.lockOpenOltManager.RLock()
		go om.OnuDeviceMap[onuDeviceKey].Start()
		om.lockOpenOltManager.RUnlock()

	}
}

func (om *OpenOltManager) stringifySerialNumber(serialNum *oop.SerialNumber) string {
	if serialNum != nil {
		return string(serialNum.VendorId) + om.stringifyVendorSpecific(serialNum.VendorSpecific)
	}
	return ""
}

func (om *OpenOltManager) stringifyVendorSpecific(vendorSpecific []byte) string {
	tmp := fmt.Sprintf("%x", (uint32(vendorSpecific[0])>>4)&0x0f) +
		fmt.Sprintf("%x", uint32(vendorSpecific[0]&0x0f)) +
		fmt.Sprintf("%x", (uint32(vendorSpecific[1])>>4)&0x0f) +
		fmt.Sprintf("%x", (uint32(vendorSpecific[1]))&0x0f) +
		fmt.Sprintf("%x", (uint32(vendorSpecific[2])>>4)&0x0f) +
		fmt.Sprintf("%x", (uint32(vendorSpecific[2]))&0x0f) +
		fmt.Sprintf("%x", (uint32(vendorSpecific[3])>>4)&0x0f) +
		fmt.Sprintf("%x", (uint32(vendorSpecific[3]))&0x0f)
	return tmp
}

// readIndications to read the indications from the OLT device
func (om *OpenOltManager) readIndications() {
	defer logger.Errorw(nil, "Indications ended", log.Fields{})
	indications, err := om.openOltClient.EnableIndication(context.Background(), new(oop.Empty))
	if err != nil {
		logger.Errorw(nil, "Failed to read indications", log.Fields{"err": err})
		return
	}
	if indications == nil {
		logger.Errorw(nil, "Indications is nil", log.Fields{})
		return
	}

	// Create an exponential backoff around re-enabling indications. The
	// maximum elapsed time for the back off is set to 0 so that we will
	// continue to retry. The max interval defaults to 1m, but is set
	// here for code clarity
	indicationBackoff := backoff.NewExponentialBackOff()
	indicationBackoff.MaxElapsedTime = 0
	indicationBackoff.MaxInterval = 1 * time.Minute
	for {
		indication, err := indications.Recv()
		if err == io.EOF {
			logger.Infow(nil, "EOF for  indications", log.Fields{"err": err})
			// Use an exponential back off to prevent getting into a tight loop
			duration := indicationBackoff.NextBackOff()
			if duration == backoff.Stop {
				// If we reach a maximum then warn and reset the backoff
				// timer and keep attempting.
				logger.Warnw(nil, "Maximum indication backoff reached, resetting backoff timer",
					log.Fields{"max_indication_backoff": indicationBackoff.MaxElapsedTime})
				indicationBackoff.Reset()
			}
			time.Sleep(indicationBackoff.NextBackOff())
			indications, err = om.openOltClient.EnableIndication(context.Background(), new(oop.Empty))
			if err != nil {
				logger.Errorw(nil, "Failed to read indications", log.Fields{"err": err})
				return
			}
			continue
		}
		if err != nil {
			logger.Infow(nil, "Failed to read from indications", log.Fields{"err": err})
			break
		}
		// Reset backoff if we have a successful receive
		indicationBackoff.Reset()
		om.handleIndication(indication)

	}
}

func (om *OpenOltManager) handleIndication(indication *oop.Indication) {
	switch indication.Data.(type) {
	case *oop.Indication_OltInd:
		logger.Info(nil, "received olt indication")
	case *oop.Indication_IntfInd:
		intfInd := indication.GetIntfInd()
		logger.Infow(nil, "Received interface indication ", log.Fields{"InterfaceInd": intfInd})
	case *oop.Indication_IntfOperInd:
		intfOperInd := indication.GetIntfOperInd()
		if intfOperInd.GetType() == "nni" {
			logger.Info(nil, "received interface oper indication for nni port")
		} else if intfOperInd.GetType() == "pon" {
			logger.Info(nil, "received interface oper indication for pon port")
		}
		/*
			case *oop.Indication_OnuDiscInd:
				onuDiscInd := indication.GetOnuDiscInd()
				logger.Infow(nil, "Received Onu discovery indication ", log.Fields{"OnuDiscInd": onuDiscInd})
		*/
	case *oop.Indication_OnuInd:
		onuInd := indication.GetOnuInd()
		logger.Infow(nil, "Received Onu indication ", log.Fields{"OnuInd": onuInd})
	case *oop.Indication_OmciInd:
		omciInd := indication.GetOmciInd()
		logger.Debugw(nil, "Received Omci indication ", log.Fields{"IntfId": omciInd.IntfId, "OnuId": omciInd.OnuId, "pkt": hex.EncodeToString(omciInd.Pkt)})
	case *oop.Indication_PktInd:
		pktInd := indication.GetPktInd()
		logger.Infow(nil, "Received packet indication ", log.Fields{"PktInd": pktInd})
		/*
				case *oop.Indication_PortStats:
				portStats := indication.GetPortStats()
				logger.Infow(nil, "Received port stats", log.Fields{"portStats": portStats})
			case *oop.Indication_FlowStats:
				flowStats := indication.GetFlowStats()
				logger.Infow(nil, "Received flow stats", log.Fields{"FlowStats": flowStats})
		*/
	case *oop.Indication_AlarmInd:
		alarmInd := indication.GetAlarmInd()
		logger.Infow(nil, "Received alarm indication ", log.Fields{"AlarmInd": alarmInd})
	}
}

func (om *OpenOltManager) populateTechProfilePerPonPort() error {
	var tpCount int
	for _, techRange := range om.deviceInfo.Ranges {
		for _, intfID := range techRange.IntfIds {
			om.TechProfile[intfID] = &(om.rsrMgr.ResourceMgrs[intfID].TechProfileMgr)
			tpCount++
			logger.Debugw(nil, "Init tech profile done", log.Fields{"intfID": intfID})
		}
	}
	//Make sure we have as many tech_profiles as there are pon ports on the device
	if tpCount != int(om.deviceInfo.GetPonPorts()) {
		logger.Errorw(nil, "Error while populating techprofile",
			log.Fields{"numofTech": tpCount, "numPonPorts": om.deviceInfo.GetPonPorts()})
		return errors.New("error while populating techprofile mgrs")
	}
	logger.Infow(nil, "Populated techprofile for ponports successfully",
		log.Fields{"numofTech": tpCount, "numPonPorts": om.deviceInfo.GetPonPorts()})
	return nil
}

func isPowerOfTwo(numOfOnus uint) bool {
	return (numOfOnus & (numOfOnus - 1)) == 0
}
