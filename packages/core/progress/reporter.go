package progress

import "time"

// Reporter allows external code to receive progress updates during workflow execution.
// CLI implements this with TUI updates, API could implement with webhooks/SSE.
type Reporter interface {
	OnPrepareStart(workflow string)
	OnPrepareProgress(step string, current, total int)
	OnPrepareComplete(workflow string)
	OnRunStart(job string)
	OnRunOutput(line string)
	OnRunComplete(job string, success bool, duration time.Duration)
	OnError(err error)
}

// NoOp is a Reporter that does nothing. Use as default when no reporting is needed.
type NoOp struct{}

func (NoOp) OnPrepareStart(workflow string)                              {}
func (NoOp) OnPrepareProgress(step string, current, total int)           {}
func (NoOp) OnPrepareComplete(workflow string)                           {}
func (NoOp) OnRunStart(job string)                                       {}
func (NoOp) OnRunOutput(line string)                                     {}
func (NoOp) OnRunComplete(job string, success bool, duration time.Duration) {}
func (NoOp) OnError(err error)                                           {}
