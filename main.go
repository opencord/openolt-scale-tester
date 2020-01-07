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

//Package main invokes the application
package main

import (
	"fmt"
	"github.com/opencord/openolt-scale-tester/core"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/opencord/openolt-scale-tester/config"
	"github.com/opencord/voltha-lib-go/v2/pkg/log"
)

func init() {
	_, _ = log.AddPackage(log.JSON, log.DebugLevel, nil)
}

const (
	// DefaultLivelinessCheck to check the liveliness
	DefaultLivelinessCheck = 30 * time.Second
	// DefaultTimeout to close the connection
	DefaultTimeout = 10
)

type OpenOltScaleTester struct {
	openOltManager *core.OpenOltManager
}

func waitForExit() int {
	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	exitChannel := make(chan int)

	go func() {
		s := <-signalChannel
		switch s {
		case syscall.SIGHUP,
			syscall.SIGINT,
			syscall.SIGTERM,
			syscall.SIGQUIT:
			log.Infow("closing-signal-received", log.Fields{"signal": s})
			exitChannel <- 0
		default:
			log.Infow("unexpected-signal-received", log.Fields{"signal": s})
			exitChannel <- 1
		}
	}()

	code := <-exitChannel
	return code
}

func printBanner() {
	fmt.Println("TODO: Print banner here")
}

func main() {
	start := time.Now()
	sc := OpenOltScaleTester{}
	cf := config.NewOpenOltScaleTesterConfig()
	cf.ParseCommandArguments()
	// Generate TP ID List from TP ID string parsed from command line for a given subscriber
	cf.TpIDList = config.GetTpIDList(cf.TpIDsString)
	sc.openOltManager = core.NewOpenOltManager(cf.OpenOltAgentAddress)

	printBanner()

	// Setup logging

	// Setup default logger - applies for packages that do not have specific logger set
	if _, err := log.SetDefaultLogger(log.JSON, 0, log.Fields{"instanceId": 0}); err != nil {
		log.With(log.Fields{"error": err}).Fatal("Cannot setup logging")
	}

	// Update all loggers (provisioned via init) with a common field
	if err := log.UpdateAllLoggers(log.Fields{"instanceId": 0}); err != nil {
		log.With(log.Fields{"error": err}).Fatal("Cannot setup logging")
	}

	log.SetPackageLogLevel("github.com/opencord/voltha-lib-go/v2/pkg/adapters/common", log.DebugLevel)

	log.Infow("config", log.Fields{"config": *cf})

	go sc.openOltManager.Start(cf)

	code := waitForExit()
	log.Infow("received-a-closing-signal", log.Fields{"code": code})

	elapsed := time.Since(start)
	log.Infow("run-time", log.Fields{"instanceId": 0, "time": elapsed / time.Second})
}
