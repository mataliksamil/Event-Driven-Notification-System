//go:build integration

package integration

import (
	"fmt"
	"net/http"

	"github.com/samil/notification/internal/worker"
)

func (s *IntegrationSuite) TestBatchSuccess() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551234567", "channel": "sms", "content": "Hello"},
			{"recipient": "user@example.com", "channel": "email", "content": "World", "priority": "high"},
		},
	}

	resp := s.post(s.batchURL, key, payload)
	defer resp.Body.Close()

	s.Equal(http.StatusAccepted, resp.StatusCode)
	result := s.decodeJSON(resp)
	s.NotEmpty(result["batch_id"])
	s.Equal("accepted", result["status"])
	s.Equal(float64(2), result["total_count"])
	s.NotEmpty(result["accepted_at"])
}

func (s *IntegrationSuite) TestBatchMissingIdempotencyKey() {
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551234567", "channel": "sms", "content": "Hello"},
		},
	}

	resp := s.post(s.batchURL, "", payload)
	defer resp.Body.Close()

	s.Equal(http.StatusBadRequest, resp.StatusCode)
	result := s.decodeJSON(resp)
	s.Contains(result["error"], "Idempotency-Key")
}

func (s *IntegrationSuite) TestBatchInvalidIdempotencyKey() {
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551234567", "channel": "sms", "content": "Hello"},
		},
	}

	resp := s.post(s.batchURL, "not-a-uuid", payload)
	defer resp.Body.Close()

	s.Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *IntegrationSuite) TestBatchEmptyNotifications() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{},
	}

	resp := s.post(s.batchURL, key, payload)
	defer resp.Body.Close()

	s.Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *IntegrationSuite) TestBatchExceedsMaxNotifications() {
	key := newUUID()
	notifications := make([]map[string]string, 1001)
	for i := range notifications {
		notifications[i] = map[string]string{
			"recipient": fmt.Sprintf("user%d@test.com", i),
			"channel":   "email",
			"content":  "Test",
		}
	}
	payload := map[string]interface{}{"notifications": notifications}

	resp := s.post(s.batchURL, key, payload)
	defer resp.Body.Close()

	s.Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *IntegrationSuite) TestBatchInvalidChannel() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551234567", "channel": "fax", "content": "Hello"},
		},
	}

	resp := s.post(s.batchURL, key, payload)
	defer resp.Body.Close()

	s.Equal(http.StatusBadRequest, resp.StatusCode)
	result := s.decodeJSON(resp)
	s.Contains(result["error"], "invalid channel")
}

func (s *IntegrationSuite) TestBatchInvalidPriority() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551234567", "channel": "sms", "content": "Hello", "priority": "urgent"},
		},
	}

	resp := s.post(s.batchURL, key, payload)
	defer resp.Body.Close()

	s.Equal(http.StatusBadRequest, resp.StatusCode)
	result := s.decodeJSON(resp)
	s.Contains(result["error"], "invalid priority")
}

func (s *IntegrationSuite) TestBatchMissingRequiredFields() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"channel": "sms", "content": "Hello"},
		},
	}

	resp := s.post(s.batchURL, key, payload)
	defer resp.Body.Close()

	s.Equal(http.StatusBadRequest, resp.StatusCode)
	result := s.decodeJSON(resp)
	s.Contains(result["error"], "required")
}

func (s *IntegrationSuite) TestBatchDuplicateIdempotencyKey_ReturnsCachedResponse() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551234567", "channel": "sms", "content": "Hello"},
		},
	}

	resp1 := s.post(s.batchURL, key, payload)
	defer resp1.Body.Close()
	s.Equal(http.StatusAccepted, resp1.StatusCode)
	result1 := s.decodeJSON(resp1)
	batchID := result1["batch_id"]

	resp2 := s.post(s.batchURL, key, payload)
	defer resp2.Body.Close()
	s.Equal(http.StatusAccepted, resp2.StatusCode)
	result2 := s.decodeJSON(resp2)
	s.Equal(batchID, result2["batch_id"])
	s.Equal("accepted", result2["status"])
}

func (s *IntegrationSuite) TestBatchProcessingStatus() {
	key := newUUID()

	err := s.redisClient.Set(ctx(), "idempotency:"+key, "processing", 0).Err()
	s.Require().NoError(err)

	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551234567", "channel": "sms", "content": "Hello"},
		},
	}

	resp := s.post(s.batchURL, key, payload)
	defer resp.Body.Close()

	s.Equal(http.StatusAccepted, resp.StatusCode)
	result := s.decodeJSON(resp)
	s.Equal("processing", result["status"])
}

func (s *IntegrationSuite) TestBatchErrorReleasesIdempotencyKey() {
	key := newUUID()

	badPayload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551234567", "channel": "fax", "content": "Hello"},
		},
	}

	resp := s.post(s.batchURL, key, badPayload)
	defer resp.Body.Close()
	s.Equal(http.StatusBadRequest, resp.StatusCode)

	validPayload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551234567", "channel": "sms", "content": "Hello"},
		},
	}

	resp2 := s.post(s.batchURL, key, validPayload)
	defer resp2.Body.Close()
	s.Equal(http.StatusAccepted, resp2.StatusCode)
}

func (s *IntegrationSuite) TestBatchDefaultPriorityIsNormal() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551234567", "channel": "sms", "content": "Hello"},
		},
	}

	resp := s.post(s.batchURL, key, payload)
	defer resp.Body.Close()
	s.Equal(http.StatusAccepted, resp.StatusCode)

	var priority string
	err := queryRow(ctx(), "SELECT priority FROM notifications LIMIT 1", &priority)
	s.Require().NoError(err)
	s.Equal("normal", priority)
}

func (s *IntegrationSuite) TestBatchEnqueue_SmsTaskInDefaultQueue() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551234567", "channel": "sms", "content": "Hello"},
		},
	}

	resp := s.post(s.batchURL, key, payload)
	defer resp.Body.Close()
	s.Equal(http.StatusAccepted, resp.StatusCode)

	tasks := s.pendingTasks("default")
	s.Len(tasks, 1)
	s.Equal(worker.TaskDeliverySMS, tasks[0].Type)

	var p worker.NotificationPayload
	s.Require().NoError(jsonUnmarshal(tasks[0].Payload, &p))
	s.NotEmpty(p.NotificationID)
}

func (s *IntegrationSuite) TestBatchEnqueue_EmailTaskInDefaultQueue() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "user@example.com", "channel": "email", "content": "Hello"},
		},
	}

	resp := s.post(s.batchURL, key, payload)
	defer resp.Body.Close()
	s.Equal(http.StatusAccepted, resp.StatusCode)

	tasks := s.pendingTasks("default")
	s.Len(tasks, 1)
	s.Equal(worker.TaskDeliveryEmail, tasks[0].Type)
}

func (s *IntegrationSuite) TestBatchEnqueue_PushTaskInDefaultQueue() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "device123", "channel": "push", "content": "Hello"},
		},
	}

	resp := s.post(s.batchURL, key, payload)
	defer resp.Body.Close()
	s.Equal(http.StatusAccepted, resp.StatusCode)

	tasks := s.pendingTasks("default")
	s.Len(tasks, 1)
	s.Equal(worker.TaskDeliveryPush, tasks[0].Type)
}

func (s *IntegrationSuite) TestBatchEnqueue_HighPriorityInCriticalQueue() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551234567", "channel": "sms", "content": "Urgent", "priority": "high"},
		},
	}

	resp := s.post(s.batchURL, key, payload)
	defer resp.Body.Close()
	s.Equal(http.StatusAccepted, resp.StatusCode)

	critical := s.pendingTasks("critical")
	s.Len(critical, 1)
	s.Equal(worker.TaskDeliverySMS, critical[0].Type)
}

func (s *IntegrationSuite) TestBatchEnqueue_LowPriorityInLowQueue() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551234567", "channel": "sms", "content": "Low prio", "priority": "low"},
		},
	}

	resp := s.post(s.batchURL, key, payload)
	defer resp.Body.Close()
	s.Equal(http.StatusAccepted, resp.StatusCode)

	lowTasks := s.pendingTasks("low")
	s.Len(lowTasks, 1)
	s.Equal(worker.TaskDeliverySMS, lowTasks[0].Type)
}

func (s *IntegrationSuite) TestBatchEnqueue_NotificationPayloadContainsID() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551234567", "channel": "sms", "content": "Hello"},
		},
	}

	resp := s.post(s.batchURL, key, payload)
	defer resp.Body.Close()
	s.Equal(http.StatusAccepted, resp.StatusCode)

	var notificationID string
	err := queryRow(ctx(), "SELECT id FROM notifications LIMIT 1", &notificationID)
	s.Require().NoError(err)

	tasks := s.pendingTasks("default")
	s.Len(tasks, 1)

	var p worker.NotificationPayload
	s.Require().NoError(jsonUnmarshal(tasks[0].Payload, &p))
	s.Equal(notificationID, p.NotificationID.String())
}

func (s *IntegrationSuite) TestBatchEnqueue_MultipleNotificationsMultipleQueues() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551234567", "channel": "sms", "content": "Urgent", "priority": "high"},
			{"recipient": "user@example.com", "channel": "email", "content": "Normal"},
			{"recipient": "device123", "channel": "push", "content": "Low prio", "priority": "low"},
		},
	}

	resp := s.post(s.batchURL, key, payload)
	defer resp.Body.Close()
	s.Equal(http.StatusAccepted, resp.StatusCode)

	critical := s.pendingTasks("critical")
	s.Len(critical, 1)
	s.Equal(worker.TaskDeliverySMS, critical[0].Type)

	defaultTasks := s.pendingTasks("default")
	s.Len(defaultTasks, 1)
	s.Equal(worker.TaskDeliveryEmail, defaultTasks[0].Type)

	lowTasks := s.pendingTasks("low")
	s.Len(lowTasks, 1)
	s.Equal(worker.TaskDeliveryPush, lowTasks[0].Type)
}