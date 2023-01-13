// This package will fetch the recommendation from the recommendation Queue and provision the scale in/out
// based on command.
package provision

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"scaling_manager/cluster"
	"scaling_manager/config"
	"strings"
	"time"
	"os"
	"context"
	"bufio"

	"scaling_manager/logger"
)

var log = new(logger.LOG)

// This struct contains the State of the opensearch scaling manager
// States can be of following types: (May change in real implementation with new stages identified)
// At any point, the state should have either "scaleup/scaledown" to identify the current operation happening
//
//   - normal : This is the state when the recommnedation will be provisioned.
//   - provisioning_scaleup/provisioning_scaledown : Once the provision module will start provisioning it will set this state.
//   - start_scaleup_process/start_scaledown_process : Indicates start of scaleup/scaledown process
//   - scaleup_triggered_spin_vm: Indicates trigger for spinning new vms while scaleup
//   - scaledown_node_identified: A state to identify node identification to scaledown
//   - provisioning_scaleup_completed/provisioning_scaledown_completed : Once the provision is completed then this state will be state.
//   - provisioning_scaleup_failed/provisioning_scaledown_failed: If the provision is failed then this state will be set.
//   - provisioned_scaleup_successfully/provisioned_scaledown_successfully: If the provision is completed and cluster state is green then this state will be set.
type State struct {
	// CurrentState indicate the current state of the scaling manager
	CurrentState string
	// PreviousState indicates the previous state of the scaling manager
	PreviousState string
	// Remark indicates the additional remarks for the state of the scaling manager
	Remark string
	// Last Provisioned time is when the last successful provision was completed
	LastProvisionedTime time.Time
	// Start time of the current provisioning in place
	ProvisionStartTime time.Time
	// Rule triggered for provisioning. i.e., scale_up/scale_down
	RuleTriggered string
	// Rule Responsible for provisioning. i.e., cpu, mem, heap, shard, disk.
	RulesResponsible string
	// Number of nodes being added(scale_up) / removed(scale_down) from the cluster due to current provision
	NumNodes int
	// Number of nodes remaining to be scaled up/scaled down
	RemainingNodes int
}

// Initializing logger module
func init() {
	log.Init("logger")
	log.Info.Println("Provisioner module initiated")
}

// Input:
//
//	state: The current provisioning state of the system
//
// Caller: Object of ConfigClusterDetails
// Description:
//
//	TriggerProvision will scale in/out the cluster based on the operation.
//	ToDo:
//	        Think about the scenario where event based scaling needs to be performed.
//	        Morning need to scale up and evening need to scale down.
//	        If in morning the scale up was not successful then we should not perform the scale down.
//	        May be we can keep a concept of minimum number of nodes as a configuration input.
//
// Return:
func TriggerProvision(cfg config.ClusterDetails, state *State, numNodes int, operation string, RulesResponsible string) {
	state.GetCurrentState()
	if operation == "scale_up" {
		state.PreviousState = state.CurrentState
		state.CurrentState = "provisioning_scaleup"
		state.NumNodes = numNodes
		state.RemainingNodes = numNodes
		state.RuleTriggered = "scale_up"
		state.RulesResponsible = RulesResponsible
		state.UpdateState()
		isScaledUp := ScaleOut(cfg, state)
		if isScaledUp {
			log.Info.Println("Scaleup successful")
		} else {
			state.GetCurrentState()
			// Add a retry mechanism
			state.PreviousState = state.CurrentState
			state.CurrentState = "provisioning_scaleup_failed"
			state.UpdateState()
		}
	} else if operation == "scale_down" {
		state.PreviousState = state.CurrentState
		state.CurrentState = "provisioning_scaledown"
		state.NumNodes = numNodes
		state.RemainingNodes = numNodes
		state.RuleTriggered = "scale_down"
		state.RulesResponsible = RulesResponsible
		state.UpdateState()
		isScaledDown := ScaleIn(cfg, state)
		if isScaledDown {
			log.Info.Println("Scaledown successful")
		} else {
			state.GetCurrentState()
			// Add a retry mechanism
			state.PreviousState = state.CurrentState
			state.CurrentState = "provisioning_scaledown_failed"
			state.UpdateState()
		}
	}
}

// Input:
//
//	state: The current provisioning state of the system
//
// Caller: Object of ConfigClusterDetails
// Description:
//
//	ScaleOut will scale out the cluster with the number of nodes.
//	This function will invoke commands to create a VM based on cloud type.
//	Then it will configure the opensearch on newly created nodes.
//
// Return:
//
//	Return the status of scale out of the nodes.
func ScaleOut(cfg config.ClusterDetails, state *State) bool {
	// Read the current state of scaleup process and proceed with next step
	// If no stage was already set. The function returns an empty string. Then, start the scaleup process
	state.GetCurrentState()
	new_node := "10.81.1.225"
	if state.CurrentState == "provisioning_scaleup" {
		log.Info.Println("Starting scaleUp process")
		time.Sleep(time.Duration(config.PollingInterval) * time.Second)
		state.PreviousState = state.CurrentState
		state.CurrentState = "start_scaleup_process"
		state.ProvisionStartTime = time.Now()
		state.UpdateState()
	}
	// Spin new VMs based on number of nodes and cloud type
	if state.CurrentState == "start_scaleup_process" {
		log.Info.Println("Spin new vms based on the cloud type")
		time.Sleep(time.Duration(config.PollingInterval) * time.Second)
		log.Info.Println("Spinning new vms")
		time.Sleep(time.Duration(config.PollingInterval) * time.Second)
		state.PreviousState = state.CurrentState
		state.CurrentState = "scaleup_triggered_spin_vm"
		state.UpdateState()
	}
	// Add the newly added VM to the list of VMs
	// Configure OS on newly created VM
	if state.CurrentState == "scaleup_triggered_spin_vm" {
		log.Info.Println("Check if the vm creation is complete and wait till done")
		time.Sleep(time.Duration(config.PollingInterval) * time.Second)
		hostsFileName := "provision/ansible_scripts/hosts"
		username := "ubuntu"
		f, err := os.OpenFile(hostsFileName, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
		    log.Fatal.Println(err)
		}
		defer f.Close()
		clusterDetails := cluster.GetCluster(context.Background())
		dataWriter := bufio.NewWriter(f)
		dataWriter.WriteString("[current-nodes]\n")
		for _, node := range clusterDetails.NodeList {
			_, _ = dataWriter.WriteString(node.NodeName + " "+ "ansible_user="+ username + " roles=master,data,ingest ansible_private_host="+node.HostIp+"\n")
		}
		dataWriter.WriteString("[new-node]\n")
		dataWriter.WriteString(new_node+ " "+ "ansible_user="+ username + " roles=master,data,ingest ansible_private_host="+new_node+"\n")
		dataWriter.Flush()
		CallScaleUp(username, hostsFileName)
		log.Info.Println("Adding the spinned nodes into the list of vms")
		time.Sleep(time.Duration(config.PollingInterval) * time.Second)
		log.Info.Println("Configure ES")
		time.Sleep(time.Duration(config.PollingInterval) * time.Second)
		log.Info.Println("Configuring in progress")
		time.Sleep(time.Duration(config.PollingInterval) * time.Second)
		state.PreviousState = state.CurrentState
		state.CurrentState = "provisioning_scaleup_completed"
		state.UpdateState()
	}
	// Check cluster status after the configuration
	if state.CurrentState == "provisioning_scaleup_completed" {
		SimulateSharRebalancing("scaleOut", state.NumNodes)
		log.Info.Println("Waiting for the cluster to become healthy")
		time.Sleep(time.Duration(config.PollingInterval) * time.Second)
		CheckClusterHealth(state)
	}
	// Setting the state back to 'normal' irrespective of successful or failed provisioning to continue further
	state.LastProvisionedTime = time.Now()
	state.ProvisionStartTime = time.Time{}
	state.PreviousState = state.CurrentState
	state.CurrentState = "normal"
	state.RuleTriggered = ""
	state.RemainingNodes = state.RemainingNodes - 1
	state.UpdateState()
	log.Info.Println("State set back to normal")

	return true
}

// Input:
//
//	state: Pointer to the current provisioning state of the system
//
// Caller: Object of ConfigClusterDetails
// Description:
//
//	ScaleIn will scale in the cluster with the number of nodes.
//	This function will invoke commands to remove a node from opensearch cluster.
//
// Return:
//
//	Return the status of scale in of the nodes.
func ScaleIn(cfg config.ClusterDetails, state *State) bool {
	// Read the current state of scaledown process and proceed with next step
	// If no stage was already set. The function returns an empty string. Then, start the scaledown process
	state.GetCurrentState()
	if state.CurrentState == "provisioning_scaledown" {
		log.Info.Println("Staring scaleDown process")
		state.PreviousState = state.CurrentState
		state.CurrentState = "start_scaledown_process"
		state.ProvisionStartTime = time.Now()
		state.UpdateState()
	}

	// Identify the node which can be removed from the cluster.
	if state.CurrentState == "start_scaledown_process" {
		log.Info.Println("Identify the node to remove from the cluster and store the node_ip")
		time.Sleep(time.Duration(config.PollingInterval) * time.Second)
		state.PreviousState = state.CurrentState
		state.CurrentState = "scaledown_node_identified"
		state.UpdateState()
	}
	// Configure OS to tell master node that the present node is going to be removed
	if state.CurrentState == "scaledown_node_identified" {
		log.Info.Println("Configure ES to remove the node ip from cluster")
		state.PreviousState = state.CurrentState
		state.CurrentState = "provisioning_scaledown_completed"
		state.UpdateState()
		time.Sleep(time.Duration(config.PollingInterval) * time.Second)
		hostsFileName := "provision/ansible_scripts/hosts"
                username := "ubuntu"
		var remove_node string
                f, err := os.OpenFile(hostsFileName, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
                if err != nil {
                    log.Fatal.Println(err)
                }
                defer f.Close()
                clusterDetails := cluster.GetCluster(context.Background())
                dataWriter := bufio.NewWriter(f)
                dataWriter.WriteString("[current-nodes]\n")
                for _, node := range clusterDetails.NodeList {
			if !(node.IsMaster) && remove_node == ""{
				remove_node = node.HostIp
			}
                        _, _ = dataWriter.WriteString(node.NodeName + " "+ "ansible_user="+ username + " roles=master,data,ingest ansible_private_host="+node.HostIp+"\n")
                }
                dataWriter.WriteString("[remove-node]\n")
                dataWriter.WriteString(remove_node+ " "+ "ansible_user="+ username + " roles=master,data,ingest ansible_private_host="+remove_node+"\n")
                dataWriter.Flush()
		log.Info.Println("Removing node ***********************************:", remove_node )
                CallScaleDown(username, hostsFileName)

		log.Info.Println("Node removed from ES configuration")
	}
	// Wait for cluster to be in stable state(Shard rebalance)
	// Shut down the node
	if state.CurrentState == "provisioning_scaledown_completed" {
		SimulateSharRebalancing("scaleIn", state.NumNodes)
		log.Info.Println("Wait for the cluster to become healthy (in a loop of 5*12 minutes) and then proceed")
		CheckClusterHealth(state)
		log.Info.Println("Shutdown the node")
		time.Sleep(time.Duration(config.PollingInterval) * time.Second)
	}
	// Setting the state back to 'normal' irrespective of successful or failed provisioning to continue further
	state.LastProvisionedTime = time.Now()
	state.ProvisionStartTime = time.Time{}
	state.RuleTriggered = ""
	state.RemainingNodes = state.RemainingNodes - 1
	state.PreviousState = state.CurrentState
	state.CurrentState = "normal"
	state.UpdateState()
	log.Info.Println("State set back to normal")

	return true
}

// Input:
//
//	state: Pointer to the current provisioning state of the system
//
// Description:
//
//	CheckClusterHealth will check the current cluster health and also check if there are any relocating
//	shards. If the cluster status is green and there are no relocating shard then we will update the status
//	to provisioned_successfully. Else, we will wait for 3 minutes and perform this check again for 3 times.
//
// Return:
func CheckClusterHealth(state *State) {
	for i := 0; i <= 12; i++ {
		clusterDynamic := cluster.GetClusterCurrent()
		log.Debug.Println(clusterDynamic.ClusterStatus)
		if clusterDynamic.ClusterStatus == "green" {
			state.GetCurrentState()
			state.PreviousState = state.CurrentState
			if strings.Contains(state.PreviousState, "scaleup") {
				state.CurrentState = "provisioned_scaleup_successfully"
			} else {
				state.CurrentState = "provisioned_scaledown_successfully"
			}
			state.UpdateState()
			break
		}
		log.Info.Println("Waiting for cluster to be healthy.......")
		time.Sleep(time.Duration(config.PollingInterval) * time.Second)
	}
	state.GetCurrentState()
	if !(strings.Contains(state.CurrentState, "success")) {
		state.PreviousState = state.CurrentState
		if strings.Contains(state.PreviousState, "scaleup") {
			state.CurrentState = "provisioning_scaleup_failed"
		} else {
			state.CurrentState = "provisioning_scaledown_failed"
		}
		state.UpdateState()
		log.Warn.Println("Cluster hasn't come back to healthy state.")
	}
	// We should wait for buffer period after provisioned_successfully state to stablize the cluster.
	// After that buffer period we should change the state to normal, which can tell trigger module to trigger
	// the recommendation.
}

func SimulateSharRebalancing(operation string, numNode int) {
	// Add logic to call the simulator's end point
	byteStr := fmt.Sprintf("{\"nodes\":%d}", numNode)
	var jsonStr = []byte(byteStr)
	var urlLink string
	if operation == "scaleOut" {
		urlLink = fmt.Sprintf("http://localhost:5000/provision/addnode")
	} else {
		urlLink = fmt.Sprintf("http://localhost:5000/provision/remnode")
	}

	req, err := http.NewRequest("POST", urlLink, bytes.NewBuffer(jsonStr))
	req.Header.Set("Content-Type", "application/json")
	client := http.Client{
		Timeout: 10 * time.Second,
	}
	resp, err := client.Do(req)

	if err != nil {
		log.Panic.Println(err)
		panic(err)
	}

	if resp.StatusCode != 200 {
		if resp.StatusCode == 404 {
			response, _ := ioutil.ReadAll(resp.Body)
			log.Error.Println(string(response))
		}
	}

	defer resp.Body.Close()
}
