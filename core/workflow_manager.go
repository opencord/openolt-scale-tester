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
	"github.com/opencord/voltha-lib-go/v7/pkg/log"
	oop "github.com/opencord/voltha-protos/v5/go/openolt"
)

type WorkFlow interface {
	ProvisionScheds(subs *Subscriber) error
	ProvisionQueues(subs *Subscriber) error
	ProvisionEapFlow(subs *Subscriber) error
	ProvisionDhcpIPV4Flow(subs *Subscriber) error
	ProvisionDhcpIPV6Flow(subs *Subscriber) error
	ProvisionIgmpFlow(subs *Subscriber) error
	ProvisionHsiaFlow(subs *Subscriber) error
	ProvisionVoipFlow(subs *Subscriber) error
	ProvisionVodFlow(subs *Subscriber) error
	ProvisionMgmtFlow(subs *Subscriber) error
	ProvisionMulticastFlow(subs *Subscriber) error
	// TODO: Add new items here as needed.
}

func DeployWorkflow(subs *Subscriber, isGroup bool) {

	defer subs.subWg.Done()

	var wf = getWorkFlow(subs)

	if isGroup {
		if err := wf.ProvisionMulticastFlow(subs); err != nil {
			subs.Reason = err.Error()
			return
		}
	} else {
		// TODO: Catch and log errors for below items if needed.
		if err := wf.ProvisionScheds(subs); err != nil {
			subs.Reason = err.Error()
			return
		}

		if err := wf.ProvisionQueues(subs); err != nil {
			subs.Reason = err.Error()
			return
		}

		if err := wf.ProvisionEapFlow(subs); err != nil {
			subs.Reason = err.Error()
			return
		}

		if err := wf.ProvisionDhcpIPV4Flow(subs); err != nil {
			subs.Reason = err.Error()
			return
		}

		if err := wf.ProvisionDhcpIPV6Flow(subs); err != nil {
			subs.Reason = err.Error()
			return
		}

		if err := wf.ProvisionIgmpFlow(subs); err != nil {
			subs.Reason = err.Error()
			return
		}

		if err := wf.ProvisionHsiaFlow(subs); err != nil {
			subs.Reason = err.Error()
			return
		}

		if err := wf.ProvisionVoipFlow(subs); err != nil {
			subs.Reason = err.Error()
			return
		}

		if err := wf.ProvisionVodFlow(subs); err != nil {
			subs.Reason = err.Error()
			return
		}

		if err := wf.ProvisionMgmtFlow(subs); err != nil {
			subs.Reason = err.Error()
			return
		}
	}

	logger.Infow(nil, "subscriber-provision-completed-from-onu-manager", log.Fields{"subsName": subs.SubscriberName})
	subs.Reason = ReasonCodeToReasonString(SUBSCRIBER_PROVISION_SUCCESS)
}

func getWorkFlow(subs *Subscriber) WorkFlow {
	switch subs.TestConfig.WorkflowName {
	case "ATT":
		logger.Info(nil, "chosen-att-workflow")
		return AttWorkFlow{}
	case "DT":
		logger.Info(nil, "chosen-dt-workflow")
		return DtWorkFlow{}
	case "TT":
		logger.Info(nil, "chosen-tt-workflow")
		return TtWorkFlow{}
	// TODO: Add new workflow here
	default:
		logger.Errorw(nil, "operator-workflow-not-supported-yet", log.Fields{"workflowName": subs.TestConfig.WorkflowName})
	}
	return nil
}

// This function should get called even before provisioning an ONUs to install trap-from-nni flows.
// The flows installed here are not related to any subscribers.
func ProvisionNniTrapFlow(oo oop.OpenoltClient, config *config.OpenOltScaleTesterConfig, rsrMgr *OpenOltResourceMgr) error {
	switch config.WorkflowName {
	case "ATT":
		if err := ProvisionAttNniTrapFlow(oo, config, rsrMgr); err != nil {
			logger.Error(nil, "error-installing-flow", log.Fields{"err": err})
			return err
		}
	case "DT":
		if err := ProvisionDtNniTrapFlow(oo, config, rsrMgr); err != nil {
			logger.Error(nil, "error-installing-flow", log.Fields{"err": err})
			return err
		}
	case "TT":
		if err := ProvisionTtNniTrapFlow(oo, config, rsrMgr); err != nil {
			logger.Error(nil, "error-installing-flow", log.Fields{"err": err})
			return err
		}
	// TODO: Add new items here
	default:
		logger.Errorw(nil, "operator-workflow-not-supported-yet", log.Fields{"workflowName": config.WorkflowName})
		return errors.New("workflow-not-supported")
	}
	return nil
}
