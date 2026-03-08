package temporal

import (
	"fmt"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"

	"groot/internal/temporal/activities"
	"groot/internal/temporal/workflows"
)

func NewWorker(c client.Client, deps activities.Dependencies) worker.Worker {
	w := worker.New(c, workflows.TaskQueueName, worker.Options{})
	w.RegisterWorkflowWithOptions(workflows.DeliveryWorkflow, workflow.RegisterOptions{Name: workflows.WorkflowName})
	w.RegisterWorkflowWithOptions(workflows.AgentWorkflow, workflow.RegisterOptions{Name: workflows.AgentWorkflowName})
	activitySet := activities.New(deps)
	w.RegisterActivityWithOptions(activitySet.DeliverHTTP, activity.RegisterOptions{Name: activities.DeliverHTTPName})
	w.RegisterActivityWithOptions(activitySet.InvokeFunction, activity.RegisterOptions{Name: activities.InvokeFunctionName})
	w.RegisterActivityWithOptions(activitySet.ExecuteConnector, activity.RegisterOptions{Name: activities.ExecuteConnectorName})
	w.RegisterActivityWithOptions(activitySet.ExecuteAgentTool, activity.RegisterOptions{Name: activities.ExecuteAgentToolName})
	w.RegisterActivity(activitySet.LoadDeliveryJob)
	w.RegisterActivity(activitySet.LoadSubscription)
	w.RegisterActivity(activitySet.LoadConnectedApp)
	w.RegisterActivity(activitySet.LoadFunctionDestination)
	w.RegisterActivity(activitySet.LoadConnectorInstance)
	w.RegisterActivity(activitySet.LoadEvent)
	w.RegisterActivity(activitySet.RecordAttempt)
	w.RegisterActivity(activitySet.MarkSucceeded)
	w.RegisterActivity(activitySet.MarkRetryableFailure)
	w.RegisterActivity(activitySet.MarkDeadLetter)
	w.RegisterActivity(activitySet.MarkFailed)
	w.RegisterActivity(activitySet.EmitResultEvent)
	w.RegisterActivity(activitySet.StartAgentRun)
	w.RegisterActivity(activitySet.LoadAgent)
	w.RegisterActivity(activitySet.ResolveAgentSession)
	w.RegisterActivity(activitySet.LinkAgentSessionEvent)
	w.RegisterActivity(activitySet.AttachAgentRunContext)
	w.RegisterActivity(activitySet.RunAgentRuntime)
	w.RegisterActivity(activitySet.RecordAgentRuntimeSteps)
	w.RegisterActivity(activitySet.UpdateAgentSessionAfterRun)
	w.RegisterActivity(activitySet.RecordAgentLLMStep)
	w.RegisterActivity(activitySet.RecordAgentToolStep)
	w.RegisterActivity(activitySet.CompleteAgentRun)
	w.RegisterActivity(activitySet.FailAgentRun)
	return w
}

func StartWorker(w worker.Worker) error {
	if err := w.Start(); err != nil {
		return fmt.Errorf("start temporal worker: %w", err)
	}
	return nil
}
