package scaleManager

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/maplelabs/opensearch-scaling-manager/config"
	"github.com/maplelabs/opensearch-scaling-manager/crypto"
	fetch "github.com/maplelabs/opensearch-scaling-manager/fetchmetrics"
	"github.com/maplelabs/opensearch-scaling-manager/logger"
	"github.com/maplelabs/opensearch-scaling-manager/provision"
	"github.com/maplelabs/opensearch-scaling-manager/recommendation"
	utils "github.com/maplelabs/opensearch-scaling-manager/utilities"

	"github.com/fsnotify/fsnotify"
	"github.com/tkuchiki/faketime"
)

// A global variable to maintain the state of current provisioning at any point by updating this in OS document.
var state = new(provision.State)

// A global logger variable used across the package for logging.
var log logger.LOG

// A global variable which lets the provision continue from where it left off if there was an abrupt stop and restart of application.
var firstExecution bool

var seed = time.Now().Unix()

// Interval in which FetchMetrics will insert the data into Opensearch
var fetchMetricsPollingInterval = 300

// Input:
//
// Description:
//
//	Initializes the main module
//	Sets the global vraible "firstExecution" to mark the start of application
//	Calls method to initialize the Opensaerch client in osutils module by reading the config file for credentials
//	Starts the fetchMetrics module to start collecting the data and dump into Opensearch (if userCfg.MonitorWithSimulator is false)
//
// Return:
func Initialize() {
	log.Init("logger")
	log.Info.Println("Main module initialized")

	firstExecution = true

	configStruct, err := config.GetConfig()
	if err != nil {
		log.Panic.Println("The recommendation can not be made as there is an error in the validation of config file.", err)
		panic(err)
	}
	provision.InitializeDocId()

	userCfg := configStruct.UserConfig

	if !userCfg.MonitorWithSimulator {
		go fetch.FetchMetrics(fetchMetricsPollingInterval, userCfg.PurgeAfter)
	}

}

// Input:
//
// Description:
//
//	The entry point for the execution of this application
//	Performs a series of operations to do the following:
//	  * Calls a goroutine to start the periodicProvisionCheck method
//	  * In a for loop in the range of a time Ticker with interval specified in the config file:
//	        # Checks if the current node is master, reads the config file, gets the recommendation from recommendation engine and triggers provisioning
//
// Return:
func Run() {
	var t = new(time.Time)
	t_now := time.Now()
	*t = time.Date(t_now.Year(), t_now.Month(), t_now.Day(), 0, 0, 0, 0, time.UTC)
	configStruct, err := config.GetConfig()
	if err != nil {
		log.Panic.Println("The recommendation can not be made as there is an error in the validation of config file.", err)
		panic(err)
	}

	go fileWatch(configStruct)

	// A periodic check if there is a change in master node to pick up incomplete provisioning
	go periodicProvisionCheck(configStruct.UserConfig.RecommendationPollingInterval, t)
	ticker := time.NewTicker(time.Duration(configStruct.UserConfig.RecommendationPollingInterval) * time.Second)
	for ; true; <-ticker.C {
		var isMaster bool
		if configStruct.UserConfig.MonitorWithSimulator {
			isMaster = true
		} else {
			isMaster = utils.CheckIfMaster(context.Background(), "")
		}
		if configStruct.UserConfig.MonitorWithSimulator && configStruct.UserConfig.IsAccelerated {
			f := faketime.NewFaketimeWithTime(*t)
			defer f.Undo()
			f.Do()
		}
		state.GetCurrentState()
		// The recommendation and provisioning should only happen on master node
		if isMaster && state.CurrentState == "normal" {
			//              if firstExecution || state.CurrentState == "normal" {
			firstExecution = false
			// This function will be responsible for parsing the config file and fill in task_details struct.
			var task config.TaskDetails
			configStruct, err := config.GetConfig()
			if err != nil {
				log.Error.Println("The recommendation can not be made as there is an error in the validation of config file.")
				log.Error.Println(err.Error())
				continue
			}
			task.Tasks = configStruct.TaskDetails
			userCfg := configStruct.UserConfig
			clusterCfg := configStruct.ClusterDetails
			metricTasks, eventTasks := recommendation.ParseTasks(task)
			if len(eventTasks.Tasks) > 0 {
				recommendation.CreateCronJob(eventTasks, clusterCfg, userCfg, t)
			}
			recommendationList := recommendation.EvaluateTask(fetchMetricsPollingInterval, userCfg.MonitorWithSimulator, userCfg.IsAccelerated, metricTasks)
			provision.GetRecommendation(recommendationList, clusterCfg, userCfg, t)
			if configStruct.UserConfig.MonitorWithSimulator && configStruct.UserConfig.IsAccelerated {
				*t = t.Add(time.Minute * 5)
			}
		}
	}
}

// Input:
//
//	pollingInterval (int): Time in seconds which is the interval between each time the check happens
//
// Description:
//
//	It periodically checks if the master node is changed and picks up if there was any ongoing provision operation
//
// Output:
func periodicProvisionCheck(pollingInterval int, t *time.Time) {
	previousMaster := utils.CheckIfMaster(context.Background(), "")
	ticker := time.NewTicker(time.Duration(pollingInterval) * time.Second)
	for ; true; <-ticker.C {
		state.GetCurrentState()
		currentMaster := utils.CheckIfMaster(context.Background(), "")
		if state.CurrentState != "normal" && currentMaster {
			if !previousMaster || firstExecution {
				//                      if firstExecution {
				firstExecution = false
				configStruct, err := config.GetConfig()
				if err != nil {
					log.Warn.Println("Unable to get Config from GetConfig()", err)
					return
				}
				if strings.Contains(state.CurrentState, "scaleup") {
					log.Debug.Println("Calling scaleOut")
					isScaledUp, err := provision.ScaleOut(configStruct.ClusterDetails, configStruct.UserConfig, t)
					if isScaledUp {
						log.Info.Println("Scaleup completed successfully")
						provision.PushToOs("Success", err)
					} else {
						log.Warn.Println("Scaleup failed", err)
						provision.PushToOs("Failed", err)
					}
					provision.SetStateBackToNormal()
				} else if strings.Contains(state.CurrentState, "scaledown") {
					log.Debug.Println("Calling scaleIn")
					isScaledDown, err := provision.ScaleIn(configStruct.ClusterDetails, configStruct.UserConfig, t)
					if isScaledDown {
						log.Info.Println("Scaledown completed successfully")
						provision.PushToOs("Success", err)
					} else {
						log.Warn.Println("Scaledown failed", err)
						provision.PushToOs("Failed", err)
					}
					provision.SetStateBackToNormal()
				}
				if configStruct.UserConfig.MonitorWithSimulator && configStruct.UserConfig.IsAccelerated {
					*t = t.Add(time.Minute * 5)
				}
			}
		}
		// Update the previousMaster for next loop
		previousMaster = currentMaster
	}
}

// This function monitors the config.yaml residing directory for any writes continuously and on
// noticing a write event, updates the encrypted creds in the config file.
func fileWatch(previousConfigStruct config.ConfigStruct) {
	//Adding file watcher to detect the change in configuration
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Error.Println("Error while creating new fileWatcher : ", err)
	}
	defer watcher.Close()
	done := make(chan bool)

	//A go routine that keeps checking for change in configuration
	go func() {
		for {
			select {
			// watch for events
			case event := <-watcher.Events:
				if strings.Contains(event.Name, config.ConfigFileName) {
					if utils.CheckIfMaster(context.Background(), "") {
						currentConfigStruct, err := config.GetConfig()
						if err != nil {
							log.Panic.Println("Error while reading config file : ", err)
							panic(err)
						}
						currOsCredentials := currentConfigStruct.ClusterDetails.OsCredentials
						prevOsCredentials := previousConfigStruct.ClusterDetails.OsCredentials
						currCloudCredentials := currentConfigStruct.ClusterDetails.CloudCredentials
						prevCloudCredentials := previousConfigStruct.ClusterDetails.CloudCredentials
						if crypto.OsCredsMismatch(currOsCredentials, prevOsCredentials) || crypto.CloudCredsMismatch(currCloudCredentials, prevCloudCredentials) {
							log.Info.Println("FILE_EVENT encountered : Creds updated")
							crypto.UpdateSecretAndEncryptCreds(false, currentConfigStruct)
							previousConfigStruct, _ = config.GetConfig()
						} else {
							log.Info.Println("FILE_EVENT encountered : Creds not updated")
						}
					} else {
						current_secret := crypto.GetEncryptionSecret()
						if crypto.EncryptionSecret != current_secret {
							log.Info.Println("Change in Creds detected")
							crypto.EncryptionSecret = current_secret
							config_struct, _ := config.GetConfig()
							crypto.DecryptCredsAndInitializeOs(config_struct)
						}
					}
				}

			case err := <-watcher.Errors:
				log.Info.Println("Error in fileWatcher: ", err)
			}
		}
	}()

	if err := watcher.Add("."); err != nil {
		log.Error.Println("Error while adding the config file changes to the fileWatcher :", err)
	}
	<-done
}

// Input:
//
// Description:
//
//	The function performs graceful shutdown of application
//	based on current state of provision.
//	It will wait till provision is completed and exits.
//
// Return:
func CleanUp() {
	log.Info.Println("Checking State before Termination")
	for {
		state.GetCurrentState()
		if state.CurrentState == "normal" || state.CurrentState == "provisioning_scaledown_completed" || state.CurrentState == "provisioning_scaleup_completed" {
			break
		}
		time.Sleep(1 * time.Second)
	}
	log.Info.Println("Exiting Scale Manager")
	os.Exit(0)
}
