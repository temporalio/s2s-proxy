// Copied from https://github.com/temporalio/temporal/tree/v1.22.2. DO NOT EDIT.

// The MIT License
//
// Copyright (c) 2020 Temporal Technologies Inc.  All rights reserved.
//
// Copyright (c) 2020 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package tasks

import (
	"github.com/temporalio/s2s-proxy/proto/1_22/api/serviceerror"

	"github.com/temporalio/s2s-proxy/proto/1_22/server/common"
)

// TODO: deprecate this method, use logger from executable.Logger() instead

func GetTransferTaskEventID(
	transferTask Task,
) int64 {
	eventID := int64(0)
	switch task := transferTask.(type) {
	case *ActivityTask:
		eventID = task.ScheduledEventID
	case *WorkflowTask:
		eventID = task.ScheduledEventID
	case *CloseExecutionTask:
		eventID = common.FirstEventID
	case *DeleteExecutionTask:
		eventID = common.FirstEventID
	case *CancelExecutionTask:
		eventID = task.InitiatedEventID
	case *SignalExecutionTask:
		eventID = task.InitiatedEventID
	case *StartChildExecutionTask:
		eventID = task.InitiatedEventID
	case *ResetWorkflowTask:
		eventID = common.FirstEventID
	case *FakeTask:
		// no-op
	default:
		panic(serviceerror.NewInternal("unknown transfer task"))
	}
	return eventID
}

func GetTimerTaskEventID(
	timerTask Task,
) int64 {
	eventID := int64(0)

	switch task := timerTask.(type) {
	case *UserTimerTask:
		eventID = task.EventID
	case *ActivityTimeoutTask:
		eventID = task.EventID
	case *WorkflowTaskTimeoutTask:
		eventID = task.EventID
	case *WorkflowBackoffTimerTask:
		eventID = common.FirstEventID
	case *ActivityRetryTimerTask:
		eventID = task.EventID
	case *WorkflowTimeoutTask:
		eventID = common.FirstEventID
	case *DeleteHistoryEventTask:
		eventID = common.FirstEventID
	case *FakeTask:
		// no-op
	default:
		panic(serviceerror.NewInternal("unknown timer task"))
	}
	return eventID
}
