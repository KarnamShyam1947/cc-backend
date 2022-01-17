package config

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ClusterCockpit/cc-jobarchive/auth"
	"github.com/ClusterCockpit/cc-jobarchive/graph/model"
	"github.com/jmoiron/sqlx"
)

var db *sqlx.DB
var lock sync.RWMutex
var uiDefaults map[string]interface{}

var Clusters []*model.Cluster

func Init(usersdb *sqlx.DB, authEnabled bool, uiConfig map[string]interface{}, jobArchive string) error {
	db = usersdb
	uiDefaults = uiConfig
	entries, err := os.ReadDir(jobArchive)
	if err != nil {
		return err
	}

	Clusters = []*model.Cluster{}
	for _, de := range entries {
		bytes, err := os.ReadFile(filepath.Join(jobArchive, de.Name(), "cluster.json"))
		if err != nil {
			return err
		}

		var cluster model.Cluster
		if err := json.Unmarshal(bytes, &cluster); err != nil {
			return err
		}

		if cluster.FilterRanges.StartTime.To.IsZero() {
			cluster.FilterRanges.StartTime.To = time.Unix(0, 0)
		}

		if cluster.Name != de.Name() {
			return fmt.Errorf("the file '%s/cluster.json' contains the clusterId '%s'", de.Name(), cluster.Name)
		}

		Clusters = append(Clusters, &cluster)
	}

	if authEnabled {
		_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS configuration (
			username varchar(255),
			key      varchar(255),
			value    varchar(255),
			PRIMARY KEY (username, key),
			FOREIGN KEY (username) REFERENCES user (username) ON DELETE CASCADE ON UPDATE NO ACTION);`)
		if err != nil {
			return err
		}
	}

	return nil
}

// Return the personalised UI config for the currently authenticated
// user or return the plain default config.
func GetUIConfig(r *http.Request) (map[string]interface{}, error) {
	lock.RLock()
	config := make(map[string]interface{}, len(uiDefaults))
	for k, v := range uiDefaults {
		config[k] = v
	}
	lock.RUnlock()

	user := auth.GetUser(r.Context())
	if user == nil {
		return config, nil
	}

	rows, err := db.Query(`SELECT key, value FROM configuration WHERE configuration.username = ?`, user.Username)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var key, rawval string
		if err := rows.Scan(&key, &rawval); err != nil {
			return nil, err
		}

		var val interface{}
		if err := json.Unmarshal([]byte(rawval), &val); err != nil {
			return nil, err
		}

		config[key] = val
	}

	return config, nil
}

// If the context does not have a user, update the global ui configuration without persisting it!
// If there is a (authenticated) user, update only his configuration.
func UpdateConfig(key, value string, ctx context.Context) error {
	user := auth.GetUser(ctx)
	if user == nil {
		lock.RLock()
		defer lock.RUnlock()

		var val interface{}
		if err := json.Unmarshal([]byte(value), &val); err != nil {
			return err
		}

		uiDefaults[key] = val
		return nil
	}

	if _, err := db.Exec(`REPLACE INTO configuration (username, key, value) VALUES (?, ?, ?)`,
		user.Username, key, value); err != nil {
		log.Printf("db.Exec: %s\n", err.Error())
		return err
	}

	return nil
}

func GetClusterConfig(cluster string) *model.Cluster {
	for _, c := range Clusters {
		if c.Name == cluster {
			return c
		}
	}
	return nil
}

func GetPartition(cluster, partition string) *model.Partition {
	for _, c := range Clusters {
		if c.Name == cluster {
			for _, p := range c.Partitions {
				if p.Name == partition {
					return p
				}
			}
		}
	}
	return nil
}

func GetMetricConfig(cluster, metric string) *model.MetricConfig {
	for _, c := range Clusters {
		if c.Name == cluster {
			for _, m := range c.MetricConfig {
				if m.Name == metric {
					return m
				}
			}
		}
	}
	return nil
}