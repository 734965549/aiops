package execution

import (
	"context"

	execapp "github.com/734965549/aiops/internal/execution/application"
	inspectionapp "github.com/734965549/aiops/internal/inspection/application"
)

// Adapter 将 Inspection 创建执行请求适配到 Execution TaskService。
type Adapter struct {
	tasks *execapp.TaskService
}

func NewAdapter(tasks *execapp.TaskService) *Adapter {
	return &Adapter{tasks: tasks}
}

func (a *Adapter) CreateAgentTask(ctx context.Context, actor inspectionapp.Actor, req inspectionapp.CreateAgentTaskRequest) (*inspectionapp.CreateAgentTaskResult, error) {
	if a == nil || a.tasks == nil {
		return nil, nil
	}
	out, err := a.tasks.Create(ctx, execapp.Actor{UserID: actor.UserID, DisplayName: actor.DisplayName}, execapp.CreateTaskInput{
		Name: req.Name, SourceType: "inspection", SourceID: req.SourceID,
		OperationType: "command", TargetType: req.TargetType, TargetID: req.TargetID,
		TargetName: req.TargetName, Environment: req.Environment, ExecutionMode: "agent",
		MediumID: req.MediumID, CommandSpecID: req.CommandSpecID, Arguments: req.Arguments,
		RiskLevel:    req.RiskLevel,
		RollbackPlan: map[string]any{"description": "只读诊断命令，无需回滚"},
	})
	if err != nil {
		return nil, err
	}
	return &inspectionapp.CreateAgentTaskResult{
		TaskID: out.TaskID, Status: out.Status, RiskLevel: out.RiskLevel,
	}, nil
}
