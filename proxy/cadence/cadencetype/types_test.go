package cadencetype

import (
	"fmt"
	"github.com/gogo/protobuf/jsonpb"
	cadence "github.com/uber/cadence-idl/go/proto/api/v1"
	"testing"
)

func TestUnmarshal(t *testing.T) {
	sss := `{
  "eventId" : "1",
  "eventTime" : "2024-12-03T19:58:05.188219Z",
  "eventType" : "EVENT_TYPE_WORKFLOW_EXECUTION_STARTED",
  "taskId" : "1048659",
  "workflowExecutionStartedEventAttributes" : {
    "workflowType" : {
      "name" : "cadence_samples.HelloWorldWorkflow"
    },
    "taskQueue" : {
      "name" : "cadence-samples-worker",
      "kind" : "TASK_QUEUE_KIND_NORMAL"
    },
    "input" : {
      "payloads" : [ {
        "metadata" : {
          "encoding" : "anNvbi9wbGFpbg=="
        },
        "data" : "eyJtZXNzYWdlIjoiVWJlciJ9"
      } ]
    },
    "workflowExecutionTimeout" : "60s",
    "workflowRunTimeout" : "60s",
    "workflowTaskTimeout" : "10s",
    "originalExecutionRunId" : "360cfdfd-458d-4de6-90a1-6aaccdd7154d",
    "identity" : "tctl@Liangs-MBP.T-mobile.com",
    "firstExecutionRunId" : "360cfdfd-458d-4de6-90a1-6aaccdd7154d",
    "attempt" : 1,
    "workflowExecutionExpirationTime" : "2024-12-03T19:59:05.188Z",
    "firstWorkflowTaskBackoff" : "0s",
    "header" : { },
    "workflowId" : "f9cd6463-fdfb-4f1e-bb10-b61dba7492e3"
  }
}`

	event := &cadence.HistoryEvent{}
	err := jsonpb.UnmarshalString(sss, event)
	fmt.Println(err)
}
