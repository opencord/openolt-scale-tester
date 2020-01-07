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

package config

import (
	"flag"
	"fmt"
	"github.com/opencord/voltha-lib-go/v2/pkg/log"
	"strconv"
	"strings"
)

// Open OLT default constants
const (
	defaultOpenOltAgentIp          = "10.90.0.114"
	defaultOpenOltAgentPort        = 9191
	defaultNumOfOnu                = 128
	defaultNumOfSubscribersPerOnu  = 1
	defaultWorkFlowName            = "ATT"
	defaultTimeIntervalBetweenSubs = 5 // in seconds
	defaultNniIntfId               = 0
	defaultKVstoreHost             = "192.168.1.11"
	defaultKVstorePort             = 2379
	defaultTpIDs                   = "64"
)

// OpenOltScaleTesterConfigConfig represents the set of configurations used by the read-write adaptercore service
type OpenOltScaleTesterConfig struct {
	// Command line parameters
	OpenOltAgentAddress     string
	OpenOltAgentIP          string
	OpenOltAgentPort        uint
	NniIntfID               uint
	NumOfOnu                uint
	SubscribersPerOnu       uint
	WorkflowName            string
	TimeIntervalBetweenSubs uint // in seconds
	KVStoreHost             string
	KVStorePort             int
	TpIDsString             string
	TpIDList                []int
}

func init() {
	_, _ = log.AddPackage(log.JSON, log.WarnLevel, nil)
}

func GetTpIDList(tpIDsStr string) []int {
	stringSlice := strings.Split(tpIDsStr, ",")
	var tpIDSlice []int
	for _, s := range stringSlice {
		if tpID, err := strconv.Atoi(s); err == nil {
			tpIDSlice = append(tpIDSlice, tpID)
		}
	}
	log.Debugw("parsed-tp-id-slice", log.Fields{"tpIDSlice": tpIDSlice})
	return tpIDSlice
}

// NewOpenOltScaleTesterConfig returns a new RWCore config
func NewOpenOltScaleTesterConfig() *OpenOltScaleTesterConfig {
	var OpenOltScaleTesterConfig = OpenOltScaleTesterConfig{ // Default values
		OpenOltAgentAddress:     defaultOpenOltAgentIp + ":" + strconv.Itoa(defaultOpenOltAgentPort),
		NumOfOnu:                defaultNumOfOnu,
		SubscribersPerOnu:       defaultNumOfSubscribersPerOnu,
		WorkflowName:            defaultWorkFlowName,
		TimeIntervalBetweenSubs: defaultTimeIntervalBetweenSubs,
		NniIntfID:               defaultNniIntfId,
		KVStoreHost:             defaultKVstoreHost,
		KVStorePort:             defaultKVstorePort,
		TpIDList:                GetTpIDList(defaultTpIDs),
	}
	return &OpenOltScaleTesterConfig
}

// ParseCommandArguments parses the arguments for OpenOltScale Tester
func (st *OpenOltScaleTesterConfig) ParseCommandArguments() {

	help := fmt.Sprintf("OpenOLT Agent IP Address")
	flag.StringVar(&(st.OpenOltAgentIP), "openolt_agent_ip_address", defaultOpenOltAgentIp, help)

	help = fmt.Sprintf("OpenOLT Agent gRPC port")
	flag.UintVar(&(st.OpenOltAgentPort), "openolt_agent_port", defaultOpenOltAgentPort, help)

	help = fmt.Sprintf("OpenOLT Agent Nni Intf Id")
	flag.UintVar(&(st.NniIntfID), "openolt_agent_nni_intf_id", defaultNniIntfId, help)

	help = fmt.Sprintf("Number of ONU")
	flag.UintVar(&(st.NumOfOnu), "num_of_onu", defaultNumOfOnu, help)

	help = fmt.Sprintf("Subscribers per ONU")
	flag.UintVar(&(st.SubscribersPerOnu), "subscribers_per_onu", defaultNumOfSubscribersPerOnu, help)

	help = fmt.Sprintf("Workflow name")
	flag.StringVar(&(st.WorkflowName), "workflow_name", defaultWorkFlowName, help)

	help = fmt.Sprintf("Time Interval Between provisioning each subscriber")
	flag.UintVar(&(st.TimeIntervalBetweenSubs), "time_interval_between_subs", defaultTimeIntervalBetweenSubs, help)

	help = fmt.Sprintf("KV store host")
	flag.StringVar(&(st.KVStoreHost), "kv_store_host", defaultKVstoreHost, help)

	help = fmt.Sprintf("KV store port")
	flag.IntVar(&(st.KVStorePort), "kv_store_port", defaultKVstorePort, help)

	help = fmt.Sprintf("Command seperated TP ID list for Workflow")
	flag.StringVar(&(st.TpIDsString), "tp_ids", defaultTpIDs, help)

	flag.Parse()

}
