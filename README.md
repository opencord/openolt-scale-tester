# OpenOLT Scale Tester
This is used for scale testing of OpenOLT Agent + BAL (or other underlying platform)

# Design
The proposed design is here https://docs.google.com/document/d/1hQxbA8jvG1BEHeeLkM5L3sYrvgJUX7z2Tk7j1sPdd1s

# How to build
```shell
make build
```

# How to run
Make sure openolt-agent and dev_mgmt_daemon are running. Then run the below command from `openolt-scale-tester` folder.

```shell
DOCKER_HOST_IP=<your-host-ip-here> OPENOLT_AGENT_IP_ADDRESS=<olt-ip-address-here> TAG=<docker-tag-here> docker-compose -f compose/openolt-scale-tester.yml up -d
```

# openolt-agent settings for testing scale
Compile openolt-agent application with `FLOW_CHECKER` compile time flag disabled and `SCALE_AND_PERF` flag enabled in  `CPPFLAGS` under `agent/Makefile.in`

# How to add new workflows
Lets say your workflow name is XYZ, and the techprofile IDs needed by your workflow is 64 (there could be more than one techprofiles too). Create `XYZ-64.json` file with your techprofile in `tech_profiles` folder.
Edit the `compose/openolt-scale-tester.yml` file to reflect your workflow name, i.e., `XYZ` for parameter `--workflow_name`.

Create `xyz_workflow.go` (see sample `att_workflow.go`) which complies to `WorkFlow` interface defined in `workflow_manager.go` file and complete the implementation.

## More than one techprofiles for your workflow

If there are more than one techprofiles needed by your workflow, then you need to create as many techprofiles with appropriate name as described above. Edit the `compose/openolt-scale-tester.yml` file, parameter `--tp_ids` with comma separated techprofile IDs.

# Configuration

Below parameters are currently configurable from `compose/openolt-scale-tester.yml` file.

```shell
openolt_agent_ip_address [default: 10.90.0.114]
openolt_agent_port [default: 9191]
openolt_agent_nni_intf_id [default: 0]
num_of_onu [default: 128]
subscribers_per_onu [default: 1]
workflow_name [default: ATT]
time_interval_between_subs [default: 5]
kv_store_host[default: 192.168.1.11]
kv_store_port [default: 2379]
tp_ids [ default: 64]
```

# Known Limitations and Issues

The following are known issues and limitations with the openolt-scale-tester tool.

* This tool configures fake ONUs which does not really exist on the PON tree. Hence all the BAL resources (if you are using a Broadcom BAL integrated agent) it continues to remain in CONFIGURED state and never move to ACTIVE. The goal is to test the scale-limits on the number of resources we can configure and see if it meets the operator requirements.
* There are timeouts seen in the openolt-agent logs during ITUPON-Alloc object creation (if you are using Broadcom BAL3.x integrated openolt-agent). Right after ITUPON-Alloc object creation, the BAL tries to sync over PLOAM channel with ONU. But, since the ONUs we configure are fake, the PLOAM channel does not exist and ITUPON-Alloc object cannot move to ACTIVE state and times-out. Since the ITUPON-Alloc object is CONFIGURED and that serves our purpose, you may ignore this error.
