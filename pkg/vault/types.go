package vault

import "time"

const (
	// ActivityEndpoint returns activity totals for a historical range.
	ActivityEndpoint = "sys/internal/counters/activity"
	// MonthlyActivityEndpoint returns the current monthly activity snapshot.
	MonthlyActivityEndpoint = "sys/internal/counters/activity/monthly"
)

// ActivityQuery controls which activity endpoint is called and which query
// parameters are forwarded to Vault.
type ActivityQuery struct {
	StartTime string
	EndTime   string
	Monthly   bool
}

func (q ActivityQuery) Endpoint() string {
	if q.Monthly {
		return MonthlyActivityEndpoint
	}

	return ActivityEndpoint
}

type activityResponse struct {
	Data activityResponseData `json:"data"`
}

type activityResponseData struct {
	ClientCounts
	StartTime   time.Time                  `json:"start_time"`
	EndTime     time.Time                  `json:"end_time"`
	Total       ClientCounts               `json:"total"`
	ByNamespace []MonthlyActivityNamespace `json:"by_namespace"`
	Months      []MonthlyActivityMonth     `json:"months"`
}

func (d activityResponseData) normalize() *MonthlyActivityData {
	counts := d.ClientCounts
	if counts == (ClientCounts{}) && d.Total != (ClientCounts{}) {
		counts = d.Total
	}

	return &MonthlyActivityData{
		ClientCounts: counts,
		StartTime:    d.StartTime,
		EndTime:      d.EndTime,
		ByNamespace:  d.ByNamespace,
		Months:       d.Months,
	}
}

// MonthlyActivityData is the normalized activity response used by the exporter
// for both the monthly and cumulative activity endpoints.
type MonthlyActivityData struct {
	ClientCounts
	StartTime   time.Time                  `json:"start_time"`
	EndTime     time.Time                  `json:"end_time"`
	ByNamespace []MonthlyActivityNamespace `json:"by_namespace"`
	Months      []MonthlyActivityMonth     `json:"months"`
}

// ClientCounts models the client activity counters returned by Vault.
type ClientCounts struct {
	EntityClients    int `json:"entity_clients"`
	NonEntityClients int `json:"non_entity_clients"`
	SecretSyncs      int `json:"secret_syncs"`
	ACMEClients      int `json:"acme_clients"`
	Clients          int `json:"clients"`
}

// MonthlyActivityNamespace is the namespace-level attribution from the monthly activity API.
type MonthlyActivityNamespace struct {
	NamespaceID   string                 `json:"namespace_id"`
	NamespacePath string                 `json:"namespace_path"`
	Counts        ClientCounts           `json:"counts"`
	Mounts        []MonthlyActivityMount `json:"mounts"`
}

// MonthlyActivityMount is the mount-level attribution from the monthly activity API.
type MonthlyActivityMount struct {
	MountPath string       `json:"mount_path"`
	MountType string       `json:"mount_type"`
	Counts    ClientCounts `json:"counts"`
}

// MonthlyActivityMonth is a month bucket from the monthly activity API.
type MonthlyActivityMonth struct {
	Timestamp  time.Time                  `json:"timestamp"`
	Counts     ClientCounts               `json:"counts"`
	Namespaces []MonthlyActivityNamespace `json:"namespaces"`
}
