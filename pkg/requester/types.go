package requester

import (
	"context"

	"github.com/bacalhau-project/bacalhau/pkg/bidstrategy"
	"github.com/bacalhau-project/bacalhau/pkg/model"
)

// Endpoint is the frontend and entry point to the requester node for the end users to submit, update and cancel jobs.
type Endpoint interface {
	// SubmitJob submits a new job to the network.
	SubmitJob(context.Context, model.JobCreatePayload) (*model.Job, error)
	// ApproveJob approves or rejects the running of a job.
	ApproveJob(context.Context, ApproveJobRequest) error
	// CancelJob cancels an existing job.
	CancelJob(context.Context, CancelJobRequest) (CancelJobResult, error)
}

// Scheduler distributes jobs to the compute nodes and tracks the executions.
type Scheduler interface {
	StartJob(context.Context, StartJobRequest) error
	CancelJob(context.Context, CancelJobRequest) (CancelJobResult, error)
}

type Queue interface {
	Scheduler

	EnqueueJob(context.Context, model.Job) error
}

// NodeDiscoverer discovers nodes in the network that are suitable to execute a job.
type NodeDiscoverer interface {
	FindNodes(ctx context.Context, job model.Job) ([]model.NodeInfo, error)
}

// NodeRanker ranks nodes based on their suitability to execute a job.
type NodeRanker interface {
	RankNodes(ctx context.Context, job model.Job, nodes []model.NodeInfo) ([]NodeRank, error)
}

// NodeRank represents a node and its rank. The higher the rank, the more preferable a node is to execute the job.
// A negative rank means the node is not suitable to execute the job.
type NodeRank struct {
	NodeInfo model.NodeInfo
	Rank     int
}

// StartJobRequest triggers the scheduling of a job.
type StartJobRequest struct {
	Job model.Job
}

type CancelJobRequest struct {
	JobID         string
	Reason        string
	UserTriggered bool
}

type CancelJobResult struct{}

type ApproveJobRequest struct {
	ClientID string
	JobID    string
	Response bidstrategy.BidStrategyResponse
}
