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
	"github.com/cenkalti/backoff/v3"
	"github.com/opencord/openolt-scale-tester/config"
	"github.com/opencord/voltha-lib-go/v2/pkg/db/kvstore"
	"github.com/opencord/voltha-lib-go/v2/pkg/log"
	"github.com/opencord/voltha-lib-go/v2/pkg/techprofile"
	oop "github.com/opencord/voltha-protos/v2/go/openolt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"sync"
	"syscall"
	"time"
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
	ipPort        string
	deviceInfo    *oop.DeviceInfo
	OnuDeviceMap  map[OnuDeviceKey]*OnuDevice `json:"onuDeviceMap"`
	TechProfile   map[uint32]*techprofile.TechProfileIf
	clientConn    *grpc.ClientConn
	openOltClient oop.OpenoltClient
	testConfig    *config.OpenOltScaleTesterConfig
	rsrMgr        *OpenOltResourceMgr
	lockRsrAlloc  sync.RWMutex
}

func init() {
	_, _ = log.AddPackage(log.JSON, log.DebugLevel, nil)
}

func NewOpenOltManager(ipPort string) *OpenOltManager {
	log.Infow("initialized openolt manager with ipPort", log.Fields{"ipPort": ipPort})
	return &OpenOltManager{
		ipPort:       ipPort,
		OnuDeviceMap: make(map[OnuDeviceKey]*OnuDevice),
		lockRsrAlloc: sync.RWMutex{},
	}
}

func (om *OpenOltManager) readAndLoadTPsToEtcd() {
	var byteValue []byte
	var err error
	// Verify that etcd is up before starting the application.
	etcdIpPort := "http://" + om.testConfig.KVStoreHost + ":" + strconv.Itoa(om.testConfig.KVStorePort)
	client, err := kvstore.NewEtcdClient(etcdIpPort, 5)
	if err != nil || client == nil {
		log.Fatal("error-initializing-etcd-client")
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
			log.Fatalw("could-not-find-tech-profile", log.Fields{"err": err, "tpFile": tpFilePath})
		}
		log.Debugw("tp-file-opened-successfully", log.Fields{"tpFile": tpFilePath})

		// read our opened json file as a byte array.
		if byteValue, err = ioutil.ReadAll(jsonFile); err != nil {
			log.Fatalw("could-not-read-tp-file", log.Fields{"err": err, "tpFile": tpFilePath})
		}

		var tp techprofile.TechProfile

		if err = json.Unmarshal(byteValue, &tp); err != nil {
			log.Fatalw("could-not-unmarshal-tp", log.Fields{"err": err, "tpFile": tpFilePath})
		} else {
			log.Infow("tp-read-from-file", log.Fields{"tp": tp, "tpFile": tpFilePath})
		}
		kvPath := fmt.Sprintf(TechProfileKVPath, om.deviceInfo.Technology, tpID)
		tpJson, err := json.Marshal(tp)
		err = client.Put(kvPath, tpJson, 2)
		if err != nil {
			log.Fatalw("tp-put-to-etcd-failed", log.Fields{"tpPath": kvPath, "err": err})
		}
		// verify the PUT succeeded.
		kvResult, err := client.Get(kvPath, 2)
		if kvResult == nil {
			log.Fatal("tp-not-found-on-kv-after-load", log.Fields{"key": kvPath, "err": err})
		} else {
			var KvTpIns techprofile.TechProfile
			var resPtr = &KvTpIns
			if value, err := kvstore.ToByte(kvResult.Value); err == nil {
				if err = json.Unmarshal(value, resPtr); err != nil {
					log.Fatal("error-unmarshal-kv-result", log.Fields{"err": err, "key": kvPath, "value": value})
				} else {
					log.Infow("verified-ok-that-tp-load-was-good", log.Fields{"tpID": tpID, "kvPath": kvPath})
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
		log.Errorw("Failed to dial device", log.Fields{"ipPort": om.ipPort, "err": err})
		return err
	}
	om.openOltClient = oop.NewOpenoltClient(om.clientConn)

	// Populate Device Info
	if deviceInfo, err := om.populateDeviceInfo(); err != nil {
		log.Error("error fetching device info", log.Fields{"err": err, "deviceInfo": deviceInfo})
		return err
	}

	// Read and load TPs to etcd.
	om.readAndLoadTPsToEtcd()

	log.Info("etcd-up-and-running--tp-loaded-successfully")

	if om.rsrMgr = NewResourceMgr("ABCD", om.testConfig.KVStoreHost+":"+strconv.Itoa(om.testConfig.KVStorePort),
		"etcd", "openolt", om.deviceInfo); om.rsrMgr == nil {
		log.Error("Error while instantiating resource manager")
		return errors.New("instantiating resource manager failed")
	}

	om.TechProfile = make(map[uint32]*techprofile.TechProfileIf)
	if err = om.populateTechProfilePerPonPort(); err != nil {
		log.Error("Error while populating tech profile mgr\n")
		return errors.New("error-loading-tech-profile-per-ponPort")
	}

	// Start reading indications
	go om.readIndications()

	// Provision OLT NNI Trap flows as needed by the Workflow
	if err = ProvisionNniTrapFlow(om.openOltClient, om.testConfig, om.rsrMgr); err != nil {
		log.Error("failed-to-add-nni-trap-flow", log.Fields{"err": err})
	}

	// Provision ONUs one by one
	go om.provisionONUs()

	return nil

}

func (om *OpenOltManager) populateDeviceInfo() (*oop.DeviceInfo, error) {
	var err error

	if om.deviceInfo, err = om.openOltClient.GetDeviceInfo(context.Background(), new(oop.Empty)); err != nil {
		log.Errorw("Failed to fetch device info", log.Fields{"err": err})
		return nil, err
	}

	if om.deviceInfo == nil {
		log.Errorw("Device info is nil", log.Fields{})
		return nil, errors.New("failed to get device info from OLT")
	}

	log.Debugw("Fetched device info", log.Fields{"deviceInfo": om.deviceInfo})

	return om.deviceInfo, nil
}

func (om *OpenOltManager) provisionONUs() {
	var numOfONUsPerPon uint
	var i, j, onuID uint32
	var err error
	oltChan := make(chan bool)
	numOfONUsPerPon = om.testConfig.NumOfOnu / uint(om.deviceInfo.PonPorts)
	if oddONUs := om.testConfig.NumOfOnu % uint(om.deviceInfo.PonPorts); oddONUs > 0 {
		log.Warnw("Odd number ONUs left out of provisioning", log.Fields{"oddONUs": oddONUs})
	}
	totalOnusToProvision := numOfONUsPerPon * uint(om.deviceInfo.PonPorts)
	log.Infow("***** all-onu-provision-started ******",
		log.Fields{"totalNumOnus": totalOnusToProvision,
			"numOfOnusPerPon": numOfONUsPerPon,
			"numOfPons":       om.deviceInfo.PonPorts})
	for i = 0; i < om.deviceInfo.PonPorts; i++ {
		for j = 0; j < uint32(numOfONUsPerPon); j++ {
			// TODO: More work with ONU provisioning
			om.lockRsrAlloc.Lock()
			sn := GenerateNextONUSerialNumber()
			om.lockRsrAlloc.Unlock()
			log.Debugw("provisioning onu", log.Fields{"onuID": j, "ponPort": i, "serialNum": sn})
			if onuID, err = om.rsrMgr.GetONUID(i); err != nil {
				log.Errorw("error getting onu id", log.Fields{"err": err})
				continue
			}
			log.Infow("onu-provision-started-from-olt-manager", log.Fields{"onuId": onuID, "ponIntf": i})
			go om.activateONU(i, onuID, sn, om.stringifySerialNumber(sn), oltChan)
			// Wait for complete ONU provision to succeed, including provisioning the subscriber
			<-oltChan
			log.Infow("onu-provision-completed-from-olt-manager", log.Fields{"onuId": onuID, "ponIntf": i})

			// Sleep for configured time before provisioning next ONU
			time.Sleep(time.Duration(om.testConfig.TimeIntervalBetweenSubs))
		}
	}
	log.Info("******** all-onu-provisioning-completed *******")

	// TODO: We need to dump the results at the end. But below json marshall does not work
	// We will need custom Marshal function.
	/*
		e, err := json.Marshal(om)
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(string(e))
	*/

	// Stop the process once the job is done
	_ = syscall.Kill(syscall.Getpid(), syscall.SIGINT)
}

func (om *OpenOltManager) activateONU(intfID uint32, onuID uint32, serialNum *oop.SerialNumber, serialNumber string, oltCh chan bool) {
	log.Debugw("activate-onu", log.Fields{"intfID": intfID, "onuID": onuID, "serialNum": serialNum, "serialNumber": serialNumber})
	// TODO: need resource manager
	var pir uint32 = 1000000
	var onuDevice = OnuDevice{
		SerialNum:     serialNumber,
		OnuID:         onuID,
		PonIntf:       intfID,
		openOltClient: om.openOltClient,
		testConfig:    om.testConfig,
		rsrMgr:        om.rsrMgr,
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
			log.Debug("ONU activation is in progress", log.Fields{"SerialNumber": serialNumber})
			oltCh <- false
		} else {
			nanos = now.UnixNano()
			milliEnd := nanos / 1000000
			onuDevice.OnuProvisionEndTime = time.Unix(0, nanos)
			onuDevice.OnuProvisionDurationInMs = milliEnd - milliStart
			log.Errorw("activate-onu-failed", log.Fields{"Onu": Onu, "err ": err})
			onuDevice.Reason = err.Error()
			oltCh <- false
		}
	} else {
		nanos = now.UnixNano()
		milliEnd := nanos / 1000000
		onuDevice.OnuProvisionEndTime = time.Unix(0, nanos)
		onuDevice.OnuProvisionDurationInMs = milliEnd - milliStart
		onuDevice.Reason = ReasonOk
		log.Infow("activated-onu", log.Fields{"SerialNumber": serialNumber})
	}

	om.OnuDeviceMap[onuDeviceKey] = &onuDevice

	// If ONU activation was success provision the ONU
	if err == nil {
		// start provisioning the ONU
		go om.OnuDeviceMap[onuDeviceKey].Start(oltCh)
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
	defer log.Errorw("Indications ended", log.Fields{})
	indications, err := om.openOltClient.EnableIndication(context.Background(), new(oop.Empty))
	if err != nil {
		log.Errorw("Failed to read indications", log.Fields{"err": err})
		return
	}
	if indications == nil {
		log.Errorw("Indications is nil", log.Fields{})
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
			log.Infow("EOF for  indications", log.Fields{"err": err})
			// Use an exponential back off to prevent getting into a tight loop
			duration := indicationBackoff.NextBackOff()
			if duration == backoff.Stop {
				// If we reach a maximum then warn and reset the backoff
				// timer and keep attempting.
				log.Warnw("Maximum indication backoff reached, resetting backoff timer",
					log.Fields{"max_indication_backoff": indicationBackoff.MaxElapsedTime})
				indicationBackoff.Reset()
			}
			time.Sleep(indicationBackoff.NextBackOff())
			indications, err = om.openOltClient.EnableIndication(context.Background(), new(oop.Empty))
			if err != nil {
				log.Errorw("Failed to read indications", log.Fields{"err": err})
				return
			}
			continue
		}
		if err != nil {
			log.Infow("Failed to read from indications", log.Fields{"err": err})
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
		log.Info("received olt indication")
	case *oop.Indication_IntfInd:
		intfInd := indication.GetIntfInd()
		log.Infow("Received interface indication ", log.Fields{"InterfaceInd": intfInd})
	case *oop.Indication_IntfOperInd:
		intfOperInd := indication.GetIntfOperInd()
		if intfOperInd.GetType() == "nni" {
			log.Info("received interface oper indication for nni port")
		} else if intfOperInd.GetType() == "pon" {
			log.Info("received interface oper indication for pon port")
		}
		/*
			case *oop.Indication_OnuDiscInd:
				onuDiscInd := indication.GetOnuDiscInd()
				log.Infow("Received Onu discovery indication ", log.Fields{"OnuDiscInd": onuDiscInd})
		*/
	case *oop.Indication_OnuInd:
		onuInd := indication.GetOnuInd()
		log.Infow("Received Onu indication ", log.Fields{"OnuInd": onuInd})
	case *oop.Indication_OmciInd:
		omciInd := indication.GetOmciInd()
		log.Debugw("Received Omci indication ", log.Fields{"IntfId": omciInd.IntfId, "OnuId": omciInd.OnuId, "pkt": hex.EncodeToString(omciInd.Pkt)})
	case *oop.Indication_PktInd:
		pktInd := indication.GetPktInd()
		log.Infow("Received pakcet indication ", log.Fields{"PktInd": pktInd})
		/*
				case *oop.Indication_PortStats:
				portStats := indication.GetPortStats()
				log.Infow("Received port stats", log.Fields{"portStats": portStats})
			case *oop.Indication_FlowStats:
				flowStats := indication.GetFlowStats()
				log.Infow("Received flow stats", log.Fields{"FlowStats": flowStats})
		*/
	case *oop.Indication_AlarmInd:
		alarmInd := indication.GetAlarmInd()
		log.Infow("Received alarm indication ", log.Fields{"AlarmInd": alarmInd})
	}
}

func (om *OpenOltManager) populateTechProfilePerPonPort() error {
	var tpCount int
	for _, techRange := range om.deviceInfo.Ranges {
		for _, intfID := range techRange.IntfIds {
			om.TechProfile[intfID] = &(om.rsrMgr.ResourceMgrs[intfID].TechProfileMgr)
			tpCount++
			log.Debugw("Init tech profile done", log.Fields{"intfID": intfID})
		}
	}
	//Make sure we have as many tech_profiles as there are pon ports on the device
	if tpCount != int(om.deviceInfo.GetPonPorts()) {
		log.Errorw("Error while populating techprofile",
			log.Fields{"numofTech": tpCount, "numPonPorts": om.deviceInfo.GetPonPorts()})
		return errors.New("error while populating techprofile mgrs")
	}
	log.Infow("Populated techprofile for ponports successfully",
		log.Fields{"numofTech": tpCount, "numPonPorts": om.deviceInfo.GetPonPorts()})
	return nil
}
