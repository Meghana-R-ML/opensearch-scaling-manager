package config

import (
	"io/ioutil"
	"os"
	"regexp"

	"github.com/go-playground/validator/v10"
	"github.com/maplelabs/opensearch-scaling-manager/cluster"
	"github.com/maplelabs/opensearch-scaling-manager/logger"
	"gopkg.in/yaml.v3"
)

var log logger.LOG
var ConfigFileName = "config.yaml"

// Input:
//
// Description:
//
//	Initialize the Config module.
//
// Return:
func init() {
	log.Init("logger")
	log.Info.Println("Config module initialized")
}

// This struct contains the OS Admin Username and OS Admin Password via which we can connect to OS cluster.
type OsCredentials struct {
	// OsAdminUsername indicates the OS Admin Username via which OS client can connect to OS Cluster.
	OsAdminUsername string `yaml:"os_admin_username" validate:"required" json:"os_admin_username"`
	// OsAdminPassword indicates the OS Admin Password via which OS client can connect to OS Cluster.
	OsAdminPassword string `yaml:"os_admin_password" validate:"required" json:"os_admin_password"`
}

// This struct contains the Cloud Secret Key and Access Key via which we can connect to the cloud.
type CloudCredentials struct {
	PemFilePath string `yaml:"pem_file_path" validate:"required" json:"pem_file_path"`
	// SecretKey indicates the Secret key for connecting to the cloud.
	SecretKey string `yaml:"secret_key" validate:"required_without=RoleArn" json:"secret_key"`
	// AccessKey indicates the Access key for connecting to the cloud.
	AccessKey string `yaml:"access_key" validate:"required_without=RoleArn" json:"access_key"`
	Region    string `yaml:"region" validate:"required" json:"region"`
	RoleArn   string `yaml:"role_arn" validate:"required_without_all=SecretKey AccessKey" json:"role_arn"`
}

// This struct contains the data structure to parse the cluster details present in the configuration file.
type ClusterDetails struct {
	// ClusterStatic indicates the static configuration for the cluster.
	cluster.ClusterStatic `yaml:",inline"`
	LaunchTemplateId      string           `yaml:"launch_template_id" validate:"required" json:"launch_template_id"`
	LaunchTemplateVersion string           `yaml:"launch_template_version" validate:"required" json:"launch_template_version"`
	SshUser               string           `yaml:"os_user" validate:"required" json:"os_user"`
	OsGroup               string           `yaml:"os_group" validate:"required" json:"os_group"`
	OpensearchVersion     string           `yaml:"os_version" validate:"required" json:"os_version"`
	OpensearchHome        string           `yaml:"os_home" validate:"required" json:"os_home"`
	DomainName            string           `yaml:"domain_name" validate:"required" json:"domain_name"`
	OsCredentials         OsCredentials    `yaml:"os_credentials" json:"os_credentials"`
	CloudCredentials      CloudCredentials `yaml:"cloud_credentials" json:"cloud_credentials"`
	JvmFactor             float64          `yaml:"jvm_factor" validate:"required,max=0.5" json:"jvm_factor"`
}

// Config for application behaviour from user
type UserConfig struct {
	MonitorWithLogs               bool `yaml:"monitor_with_logs"`
	MonitorWithSimulator          bool `yaml:"monitor_with_simulator"`
	PurgeAfter                    int  `yaml:"purge_old_docs_after_hours" validate:"required,min=1"`
	RecommendationPollingInterval int  `yaml:"recommendation_polling_interval_in_secs" validate:"required,min=60"`
	IsAccelerated                 bool `yaml:"is_accelerated"`
}

// This struct contains the data structure to parse the configuration file.
type ConfigStruct struct {
	UserConfig     UserConfig     `yaml:"user_config"`
	ClusterDetails ClusterDetails `yaml:"cluster_details"`
	TaskDetails    []Task         `yaml:"task_details" validate:"gt=0,dive"`
}

// This struct contains the task to be perforrmed by the recommendation and set of rules wrt the action.
type Task struct {
	// TaskName indicates the name of the task to recommend by the recommendation engine.
	TaskName string `yaml:"task_name" validate:"required,isValidTaskName"`
	// Rules indicates list of rules to evaluate the criteria for the recomm+endation engine.
	Rules []Rule `yaml:"rules" validate:"gt=0,dive"`
	// Operator indicates the logical operation needs to be performed while executing the rules
	Operator string `yaml:"operator" validate:"required,oneof=AND OR EVENT"`
}

// This struct contains the rule.
type Rule struct {
	// Metic indicates the name of the metric. These can be:
	//      Cpu
	//      Mem
	//      Shard
	Metric string `yaml:"metric,omitempty"`
	// Limit indicates the threshold value for a metric.
	// If this threshold is achieved for a given metric for the decision periond then the rule will be activated.
	Limit float32 `yaml:"limit,omitempty"`
	// Stat indicates the statistics on which the evaluation of the rule will happen.
	// For Cpu and Mem the values can be:
	//              Avg: The average CPU or MEM value will be calculated for a given decision period.
	//              Count: The number of occurences where CPU or MEM value crossed the threshold limit.
	//              Term:
	// For rule: Shard, the stat will not be applicable as the shard will be calculated across the cluster and is not a statistical value.
	Stat string `yaml:"stat,omitempty"`
	// DecisionPeriod indicates the time in minutes for which a rule is evalated.
	DecisionPeriod int `yaml:"decision_period,omitempty"`
	// Occurrences indicate the number of time a rule reached the threshold limit for a give decision period.
	// It will be applicable only when the Stat is set to Count.
	Occurrences int `yaml:"occurrences_percent,omitempty"`
	// Scheduling time indicates cron time expression to schedule scaling operations
	// Example:
	// SchedulingTime = "30 5 * * 1-5"
	// In the above example the cron job will run at 5:30 AM from Mon-Fri of every month
	SchedulingTime string `yaml:"scheduling_time,omitempty"`
	// NumNodesRequired specifies the integer value of number of nodes to be present in cluster for event based scaling operations
	// To be implemented.
	// NumNodesRequired int `yaml:"num_nodes_required"`
}

// This struct contains the task details which is set of actions.
type TaskDetails struct {
	// Tasks indicates list of task.
	// A task indicates what operation needs to be recommended by recommendation engine.
	// As of now tasks can be of two types:
	//
	//      scale_up_by_1
	//      scale_down_by_1
	Tasks []Task `yaml:"action" validate:"gt=0,dive"`
}

// Inputs:
//
//	path (string): The path of the configuration file.
//
// Description:
//
//	This function will be parsing the provided configuration file and populate the ConfigStruct.
func GetConfig() (ConfigStruct, error) {
	yamlConfig, err := os.Open(ConfigFileName)
	if err != nil {
		log.Panic.Println("Unable to read the config file: ", err)
		panic(err)
	}
	defer yamlConfig.Close()
	configByte, _ := ioutil.ReadAll(yamlConfig)
	var config = new(ConfigStruct)
	err = yaml.Unmarshal(configByte, &config)
	if err != nil {
		log.Panic.Println("Unmarshal Error : ", err)
		panic(err)
	}
	err = validation(*config)
	return *config, err
}

// Inputs:
//
//	config (ConfigStruct): config structure populated with unmarshalled data.
//
// Description:
//
//	This function will be validating the configuration structure.
//
// Return:
//
//	(error): Return the error if there is a validation error.
func validation(config ConfigStruct) error {
	validate := validator.New()
	validate.RegisterValidation("isValidName", isValidName)
	validate.RegisterValidation("isValidTaskName", isValidTaskName)
	validate.RegisterStructValidation(RuleStructLevelValidation, Rule{})
	err := validate.Struct(config)
	return err
}

// Inputs:
//
//	fl (validator.FieldLevel): The field which needs to be validated.
//
// Description:
//
//	This function will be validating the cluster name.
//
// Return:
//
//	(bool): Return true if there is a valid cluster name else false.
func isValidName(fl validator.FieldLevel) bool {
	nameRegexString := `^[a-zA-Z][a-zA-Z0-9\-\._]+[a-zA-Z0-9]$`
	nameRegex := regexp.MustCompile(nameRegexString)

	return nameRegex.MatchString(fl.Field().String())
}

// Inputs:
//
//	fl (validator.FieldLevel): The field which needs to be validated.
//
// Description:
//
//	This function will be validating the Task name.
//
// Return:
//
//	(bool): Return true if there is a valid Task name else false.
func isValidTaskName(fl validator.FieldLevel) bool {
	TaskNameRegexString := `scale_(up|down)_by_[0-9]+`
	TaskNameRegex := regexp.MustCompile(TaskNameRegexString)

	return TaskNameRegex.MatchString(fl.Field().String())
}

// Inputs:
//
//	fl (validator.StructLevel): The field of StructLevel needs to be validated.
//
// Description:
//
//	This function will be validating the Rule struct.
//	It will be Reporting Error when the validation for a field fails.
//
// Return:
func RuleStructLevelValidation(sl validator.StructLevel) {

	tasks := sl.Parent().Interface().(Task)
	rule := sl.Current().Interface().(Rule)

	if tasks.Operator == "AND" || tasks.Operator == "OR" {
		if rule.Stat != "COUNT" && rule.Occurrences > 0 {
			sl.ReportError(rule.Stat, "occurrences", "Occurrences", "excluded_unless", "")
		}
		if rule.Metric != "CpuUtil" && rule.Metric != "RamUtil" && rule.Metric != "DiskUtil" &&
			rule.Metric != "HeapUtil" && rule.Metric != "NumShards" && rule.Metric != "ShardsPerGB" {
			sl.ReportError(rule.Metric, "metric", "Metric", "OneOf", "")
		}
		if rule.Limit <= 0 {
			sl.ReportError(rule.Limit, "Limit", "Limit", "required", "")
		}
		if rule.Stat != "AVG" && rule.Stat != "COUNT" && rule.Stat != "TERM" {
			sl.ReportError(rule.Stat, "Stat", "Stat", "OneOf", "")
		}
		if rule.DecisionPeriod < 60 {
			sl.ReportError(rule.DecisionPeriod, "DecisionPeriod", "DecisionPeriod", "required,min", "")
		}
		if rule.Stat == "COUNT" && rule.Occurrences > 100 {
			sl.ReportError(rule.Occurrences, "Occurrences", "Occurrences", "required,max", "")
		}
	} else if tasks.Operator == "EVENT" {
		if rule.SchedulingTime == "" {
			sl.ReportError(rule.SchedulingTime, "SchedulingTime", "scheduling_time", "required", "")
		}
		// if rule.NumNodesRequired <= 0 {
		//      sl.ReportError(rule.NumNodesRequired, "NumNodesRequired", "number_of_node", "required", "")
		// }
	}
}

// Inputs:
//
//	conf (ConfigStruct) : Credentials encrypted structure of the config.yaml file
//
// Description:
//
//	This function updates the config.yaml file with encrypted credentials ConfigStruct.
//
// Return:
//
//	(error) : Error (if any), else nil
func UpdateConfigFile(conf ConfigStruct) error {
	conf_byte, err := yaml.Marshal(&conf)
	if err != nil {
		log.Error.Println("Error marshalling the ConfigStruct : ", err)
		return err
	}

	yaml_content := "---\n" + string(conf_byte)
	err = ioutil.WriteFile(ConfigFileName, []byte(yaml_content), 0)
	if err != nil {
		log.Error.Println("Error writing the config yaml file : ", err)
		return err
	}

	return nil
}
