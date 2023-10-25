// Copyright (C) 2023 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package scheduler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"time"

	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"

	openapi "github.com/ClusterCockpit/slurm-rest-client-0_0_38"
)

type SlurmRestSchedulerConfig struct {
	URL string `json:"url"`

	JobRepository *repository.JobRepository

	clusterConfig ClusterConfig
}

var client *http.Client

func queryDB(qtime int64, clusterName string) ([]interface{}, error) {

	apiEndpoint := "/slurmdb/v0.0.38/jobs"

	// Construct the query parameters
	queryParams := url.Values{}
	queryParams.Set("users", "user1,user2")
	queryParams.Set("submit_time", "2023-01-01T00:00:00")

	// Add the query parameters to the API endpoint
	apiEndpoint += "?" + queryParams.Encode()

	// Create a new HTTP GET request
	req, err := http.NewRequest("GET", apiEndpoint, nil)
	if err != nil {
		log.Errorf("Error creating request: %v", err)
	}

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		log.Errorf("Error sending request: %v", err)
	}
	defer resp.Body.Close()

	// Check the response status code
	if resp.StatusCode != http.StatusOK {
		log.Errorf("API request failed with status: %v", resp.Status)
	}

	// Read the response body
	// Here you can parse the response body as needed
	// For simplicity, let's just print the response body
	var dbOutput []byte
	_, err = resp.Body.Read(dbOutput)
	if err != nil {
		log.Errorf("Error reading response body: %v", err)
	}

	log.Errorf("API response: %v", string(dbOutput))

	dataJobs := make(map[string]interface{})
	err = json.Unmarshal(dbOutput, &dataJobs)
	if err != nil {
		log.Errorf("Error parsing JSON response: %v", err)
		os.Exit(1)
	}

	if _, ok := dataJobs["jobs"]; !ok {
		log.Errorf("ERROR: jobs not found - response incomplete")
		os.Exit(1)
	}

	jobs, _ := dataJobs["jobs"].([]interface{})
	return jobs, nil
}

func queryAllJobs() (openapi.V0038JobsResponse, error) {
	var ctlOutput []byte

	apiEndpoint := "http://:8080/slurm/v0.0.38/jobs"
	// Create a new HTTP GET request with query parameters
	req, err := http.NewRequest("GET", apiEndpoint, nil)
	if err != nil {
		log.Errorf("Error creating request: %v", err)
	}

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		log.Errorf("Error sending request: %v", err)
	}
	defer resp.Body.Close()

	// Check the response status code
	if resp.StatusCode != http.StatusOK {
		log.Errorf("API request failed with status: %v", resp.Status)
	}

	_, err = resp.Body.Read(ctlOutput)
	log.Printf("Received JSON Data: %v", ctlOutput)
	if err != nil {
		log.Errorf("Error reading response body: %v", err)
	}

	var jobsResponse openapi.V0038JobsResponse
	err = json.Unmarshal(ctlOutput, &jobsResponse)
	if err != nil {
		log.Errorf("Error parsing JSON response: %v", err)
		return jobsResponse, err
	}

	return jobsResponse, nil
}

func queryAllJobsLocal() (openapi.V0038JobsResponse, error) {
	// Read the JSON file
	jobsData, err := os.ReadFile("slurm_0038.json")

	if err != nil {
		fmt.Println("Error reading JSON file:", err)
	}

	var jobsResponse openapi.V0038JobsResponse
	err = json.Unmarshal(jobsData, &jobsResponse)
	if err != nil {
		log.Errorf("Error parsing JSON response: %v", err)
		return jobsResponse, err
	}

	return jobsResponse, nil
}

func printSlurmInfo(job openapi.V0038JobResponseProperties) string {

	text := fmt.Sprintf(`
	    JobId=%v JobName=%v
		UserId=%v(%v) GroupId=%v
		Account=%v QOS=%v
		Requeue=%v Restarts=%v BatchFlag=%v
		TimeLimit=%v
		SubmitTime=%v
		Partition=%v
		NodeList=%v
		NumNodes=%v NumCPUs=%v NumTasks=%v CPUs/Task=%v
		NTasksPerNode:Socket:Core=%v:%v:%v
		TRES_req=%v
		TRES_alloc=%v
		Command=%v
		WorkDir=%v
		StdErr=%v
		StdOut=%v`,
		job.JobId, job.Name,
		job.UserName, job.UserId, job.GroupId,
		job.Account, job.Qos,
		job.Requeue, job.RestartCnt, job.BatchFlag,
		job.TimeLimit, job.SubmitTime,
		job.Partition,
		job.Nodes,
		job.NodeCount, job.Cpus, job.Tasks, job.CpusPerTask,
		job.TasksPerBoard, job.TasksPerSocket, job.TasksPerCore,
		job.TresAllocStr,
		job.TresAllocStr,
		job.Command,
		job.CurrentWorkingDirectory,
		job.StandardError,
		job.StandardOutput,
	)

	return text
}

func exitWithError(err error, output []byte) {
	if exitError, ok := err.(*exec.ExitError); ok {
		if exitError.ExitCode() == 28 {
			fmt.Fprintf(os.Stderr, "ERROR: API call failed with timeout; check slurmrestd.\nOutput:\n%s\n", output)
		} else {
			fmt.Fprintf(os.Stderr, "ERROR: API call failed with code %d;\nOutput:\n%s\n", exitError.ExitCode(), output)
		}
	} else {
		log.Errorf("ERROR:", err)
	}
	os.Exit(1)
}

func (cfg *SlurmRestSchedulerConfig) Init() error {
	var err error

	cfg.clusterConfig, err = DecodeClusterConfig("cluster-alex.json")

	// for k, v := range cfg.clusterConfig {
	// 	fmt.Printf("Entry %q with value %x loaded\n", k, v)
	// 	// switch c := v.(type) {
	// 	// case string:
	// 	// 	fmt.Printf("Item %q is a string, containing %q\n", k, c)
	// 	// case float64:
	// 	// 	fmt.Printf("Looks like item %q is a number, specifically %f\n", k, c)
	// 	// default:
	// 	// 	fmt.Printf("Not sure what type item %q is, but I think it might be %T\n", k, c)
	// 	// }
	// }
	// fmt.Printf("Cluster Name: %q\n", cfg.clusterConfig["name"])

	// Create an HTTP client
	client = &http.Client{}

	return err
}

func (cfg *SlurmRestSchedulerConfig) checkAndHandleStopJob(job *schema.Job, req *StopJobRequest) {

	// Sanity checks
	if job == nil || job.StartTime.Unix() >= req.StopTime || job.State != schema.JobStateRunning {
		log.Errorf("stopTime must be larger than startTime and only running jobs can be stopped")
		return
	}

	if req.State != "" && !req.State.Valid() {
		log.Errorf("invalid job state: %#v", req.State)
		return
	} else if req.State == "" {
		req.State = schema.JobStateCompleted
	}

	// Mark job as stopped in the database (update state and duration)
	job.Duration = int32(req.StopTime - job.StartTime.Unix())
	job.State = req.State
	if err := cfg.JobRepository.Stop(job.ID, job.Duration, job.State, job.MonitoringStatus); err != nil {
		log.Errorf("marking job as stopped failed: %s", err.Error())
		return
	}

	log.Printf("archiving job... (dbid: %d): cluster=%s, jobId=%d, user=%s, startTime=%s", job.ID, job.Cluster, job.JobID, job.User, job.StartTime)

	// Monitoring is disabled...
	if job.MonitoringStatus == schema.MonitoringStatusDisabled {
		return
	}

	// Trigger async archiving
	cfg.JobRepository.TriggerArchiving(job)
}

func ConstructNodeAcceleratorMap(input string, accelerator string) map[string]string {
	numberMap := make(map[string]string)

	// Split the input by commas
	groups := strings.Split(input, ",")

	for _, group := range groups {
		// Use regular expressions to match numbers and ranges
		numberRangeRegex := regexp.MustCompile(`a\[(\d+)-(\d+)\]`)
		numberRegex := regexp.MustCompile(`a(\d+)`)

		if numberRangeRegex.MatchString(group) {
			// Extract nodes from ranges
			matches := numberRangeRegex.FindStringSubmatch(group)
			if len(matches) == 3 {
				start, _ := strconv.Atoi(matches[1])
				end, _ := strconv.Atoi(matches[2])
				for i := start; i <= end; i++ {
					numberMap[matches[0]+fmt.Sprintf("%04d", i)] = accelerator
				}
			}
		} else if numberRegex.MatchString(group) {
			// Extract individual node
			matches := numberRegex.FindStringSubmatch(group)
			if len(matches) == 2 {
				numberMap[group] = accelerator
			}
		}
	}

	return numberMap
}

func (cfg *SlurmRestSchedulerConfig) HandleJobsResponse(jobsResponse openapi.V0038JobsResponse) {

	// Iterate over the Jobs slice
	for _, job := range jobsResponse.Jobs {
		// Process each job
		fmt.Printf("Job ID: %d\n", *job.JobId)
		fmt.Printf("Job Name: %s\n", *job.Name)
		fmt.Printf("Job State: %s\n", *job.JobState)
		fmt.Println("Job StartTime:", *job.StartTime)
		fmt.Println("Job Cluster:", *job.Cluster)

		// aquire lock to avoid race condition between API calls
		// var unlockOnce sync.Once
		// cfg.RepositoryMutex.Lock()
		// defer unlockOnce.Do(cfg.RepositoryMutex.Unlock)

		// is "running" one of JSON state?
		if *job.JobState == "RUNNING" {

			// jobs, err := cfg.JobRepository.FindRunningJobs(*job.Cluster)
			// if err != nil {
			// 	log.Fatalf("Failed to find running jobs: %v", err)
			// }

			// for id, job := range jobs {
			// 	fmt.Printf("Job ID: %d, Job: %+v\n", id, job)
			// }

			// if err != nil || err != sql.ErrNoRows {
			// 	log.Errorf("checking for duplicate failed: %s", err.Error())
			// 	return
			// } else if err == nil {
			// 	if len(jobs) == 0 {
			var exclusive int32
			if job.Shared == nil {
				exclusive = 1
			} else {
				exclusive = 0
			}

			jobResourcesInBytes, err := json.Marshal(*job.JobResources)
			if err != nil {
				log.Fatalf("JobResources JSON marshaling failed: %s", err)
			}

			var resources []*schema.Resource

			// Define a regular expression to match "gpu=x"
			regex := regexp.MustCompile(`gpu=(\d+)`)

			// Find all matches in the input string
			matches := regex.FindAllStringSubmatch(*job.TresAllocStr, -1)

			// Initialize a variable to store the total number of GPUs
			var totalGPUs int32
			// Iterate through the matches
			match := matches[0]
			if len(match) == 2 {
				gpuCount, _ := strconv.Atoi(match[1])
				totalGPUs += int32(gpuCount)
			}

			for _, node := range job.JobResources.AllocatedNodes {
				var res schema.Resource
				res.Hostname = *node.Nodename

				log.Debugf("Node %s V0038NodeAllocationSockets.Cores map size: %d\n", *node.Nodename, len(node.Sockets.Cores))

				if node.Cpus == nil || node.Memory == nil {
					log.Fatalf("Either node.Cpus or node.Memory is nil\n")
				}

				for k, v := range node.Sockets.Cores {
					fmt.Printf("core id[%s] value[%s]\n", k, v)
					threadID, _ := strconv.Atoi(k)
					res.HWThreads = append(res.HWThreads, threadID)
				}

				// cpu=512,mem=1875G,node=4,billing=512,gres\/gpu=32,gres\/gpu:a40=32
				// For core/GPU id mapping, need to query from cluster config file
				res.Accelerators = append(res.Accelerators, *job.Comment)
				resources = append(resources, &res)
			}

			metaData := make(map[string]string)
			metaData["jobName"] = *job.Name
			metaData["slurmInfo"] = printSlurmInfo(job)

			// switch slurmPath := cfg.clusterConfig["slurm_path"].(type) {
			// case string:
			// 	commandCtlScriptTpl := fmt.Sprintf("%sscontrol -M %%s write batch_script %%s -", slurmPath)
			// 	queryJobScript := fmt.Sprintf(commandCtlScriptTpl, job.Cluster, job.JobId)
			// 	metaData["jobScript"] = queryJobScript
			// default:
			// 	// Type assertion failed
			// 	fmt.Println("Conversion of slurm_path to string failed", cfg.clusterConfig["slurm_path"])
			// }

			metaDataInBytes, err := json.Marshal(metaData)
			if err != nil {
				log.Fatalf("metaData JSON marshaling failed: %s", err)
			}

			var defaultJob schema.BaseJob = schema.BaseJob{
				JobID:     int64(*job.JobId),
				User:      *job.UserName,
				Project:   *job.Account,
				Cluster:   *job.Cluster,
				Partition: *job.Partition,
				// check nil
				ArrayJobId:   int64(*job.ArrayJobId),
				NumNodes:     *job.NodeCount,
				NumHWThreads: *job.Cpus,
				NumAcc:       totalGPUs,
				Exclusive:    exclusive,
				// MonitoringStatus: job.MonitoringStatus,
				// SMT:            *job.TasksPerCore,
				State: schema.JobState(*job.JobState),
				// ignore this for start job
				// Duration:       int32(time.Now().Unix() - *job.StartTime), // or SubmitTime?
				Walltime: time.Now().Unix(), // max duration requested by the job
				// Tags:           job.Tags,
				// ignore this!
				RawResources: jobResourcesInBytes,
				// "job_resources": "allocated_nodes" "sockets":
				// very important; has to be right
				Resources:   resources,
				RawMetaData: metaDataInBytes,
				// optional metadata with'jobScript 'jobName': 'slurmInfo':
				MetaData: metaData,
				// ConcurrentJobs: job.ConcurrentJobs,
			}
			log.Debugf("Generated BaseJob with Resources=%v", defaultJob.Resources[0])

			req := &schema.JobMeta{
				BaseJob:    defaultJob,
				StartTime:  *job.StartTime,
				Statistics: make(map[string]schema.JobStatistics),
			}
			log.Debugf("Generated JobMeta %v", req.BaseJob.JobID)

			// req := new(schema.JobMeta)
			// id, err := cfg.JobRepository.Start(req)
			// log.Debugf("Added %v", id)
			// } else {
			// 	for _, job := range jobs {
			// 		log.Errorf("a job with that jobId, cluster and startTime already exists: dbid: %d", job.ID)
			// 	}
			// }
			// }
		} else {
			// Check if completed job with combination of (job_id, cluster_id, start_time) already exists:
			var jobID int64
			jobID = int64(*job.JobId)
			log.Debugf("jobID: %v Cluster: %v StartTime: %v", jobID, *job.Cluster, *job.StartTime)
			// commented out as it will cause panic
			// note down params invoked

			existingJob, err := cfg.JobRepository.Find(&jobID, job.Cluster, job.StartTime)

			if err == nil {
				existingJob.BaseJob.Duration = int32(*job.EndTime - *job.StartTime)
				existingJob.BaseJob.State = schema.JobState(*job.JobState)
				existingJob.BaseJob.Walltime = *job.StartTime

				req := &StopJobRequest{
					Cluster:   job.Cluster,
					JobId:     &jobID,
					State:     schema.JobState(*job.JobState),
					StartTime: &existingJob.StartTimeUnix,
					StopTime:  *job.EndTime,
				}
				// req := new(schema.JobMeta)
				cfg.checkAndHandleStopJob(existingJob, req)
			}

		}
	}
}

func (cfg *SlurmRestSchedulerConfig) Sync() {

	// Fetch an instance of V0037JobsResponse
	jobsResponse, err := queryAllJobsLocal()
	if err != nil {
		log.Fatal(err.Error())
	}
	cfg.HandleJobsResponse(jobsResponse)

}
