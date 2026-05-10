//go:build integration

package integration

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/samil/notification/internal/worker"
)

func (s *IntegrationSuite) TestBatchDelivery_AllNotificationsDelivered() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551111111", "channel": "sms", "content": "SMS delivery test"},
			{"recipient": "user@example.com", "channel": "email", "content": "Email delivery test"},
			{"recipient": "device456", "channel": "push", "content": "Push delivery test"},
		},
	}

	resp := s.post(s.batchURL, key, payload)
	defer resp.Body.Close()
	s.Equal(http.StatusAccepted, resp.StatusCode)
	result := s.decodeJSON(resp)
	batchID := result["batch_id"].(string)
	s.NotEmpty(batchID)

	notificationIDs := s.getNotificationIDs(batchID)
	s.Len(notificationIDs, 3)

	for i, id := range notificationIDs {
		var taskType string
		switch i {
		case 0:
			taskType = worker.TaskDeliverySMS
		case 1:
			taskType = worker.TaskDeliveryEmail
		case 2:
			taskType = worker.TaskDeliveryPush
		}
		notificationUUID, err := uuid.Parse(id)
		s.Require().NoError(err)

		err = s.processTask(taskType, notificationUUID)
		s.Require().NoError(err)

		status := s.getNotificationStatus(id)
		s.Equal("delivered", status)
	}

	resp2 := s.get(s.notificationURL + "/" + notificationIDs[0])
	defer resp2.Body.Close()
	s.Equal(http.StatusOK, resp2.StatusCode)
	n := s.decodeJSON(resp2)
	s.Equal("delivered", n["status"])
	s.Equal("+905551111111", n["recipient"])
	s.Equal("sms", n["channel"])
	s.Equal("SMS delivery test", n["content"])
	s.Equal("normal", n["priority"])
}

func (s *IntegrationSuite) TestBatchDelivery_NotificationCancelledDuringPending() {
	key := newUUID()
	batch2Key := newUUID()

	payload1 := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905552222222", "channel": "sms", "content": "Will be cancelled"},
			{"recipient": "deliver@example.com", "channel": "email", "content": "Will be delivered"},
		},
	}

	resp := s.post(s.batchURL, key, payload1)
	defer resp.Body.Close()
	s.Equal(http.StatusAccepted, resp.StatusCode)
	result := s.decodeJSON(resp)
	batchID := result["batch_id"].(string)

	notificationIDs := s.getNotificationIDs(batchID)
	s.Len(notificationIDs, 2)
	cancelID := notificationIDs[0]
	deliverID := notificationIDs[1]

	s.mockWebhook.SetResponse(http.StatusInternalServerError, "temporary error")

	cancelUUID, err := uuid.Parse(cancelID)
	s.Require().NoError(err)
	_ = s.processTask(worker.TaskDeliverySMS, cancelUUID)

	status := s.getNotificationStatus(cancelID)
	s.Equal("pending", status)

	cancelResp := s.cancel(s.notificationURL + "/" + cancelID + "/cancel")
	defer cancelResp.Body.Close()
	s.Equal(http.StatusOK, cancelResp.StatusCode)
	cancelResult := s.decodeJSON(cancelResp)
	s.Equal("cancelled", cancelResult["status"])

	status = s.getNotificationStatus(cancelID)
	s.Equal("cancelled", status)

	s.mockWebhook.SetResponse(http.StatusOK, `{"status":"delivered"}`)
	err = s.processTask(worker.TaskDeliverySMS, cancelUUID)
	s.Require().NoError(err)

	status = s.getNotificationStatus(cancelID)
	s.Equal("cancelled", status)

	deliverUUID, err := uuid.Parse(deliverID)
	s.Require().NoError(err)
	err = s.processTask(worker.TaskDeliveryEmail, deliverUUID)
	s.Require().NoError(err)

	status = s.getNotificationStatus(deliverID)
	s.Equal("delivered", status)

	payload2 := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905553333333", "channel": "sms", "content": "Another batch"},
		},
	}
	resp2 := s.post(s.batchURL, batch2Key, payload2)
	defer resp2.Body.Close()
	s.Equal(http.StatusAccepted, resp2.StatusCode)
	batch2Result := s.decodeJSON(resp2)
	batch2ID := batch2Result["batch_id"].(string)
	s.NotEmpty(batch2ID)
	s.NotEqual(batchID, batch2ID)
}

func (s *IntegrationSuite) TestBatchDelivery_PermanentFailureMarksFailed() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905554444444", "channel": "sms", "content": "Permanent fail"},
		},
	}

	resp := s.post(s.batchURL, key, payload)
	defer resp.Body.Close()
	s.Equal(http.StatusAccepted, resp.StatusCode)
	result := s.decodeJSON(resp)
	batchID := result["batch_id"].(string)

	notificationIDs := s.getNotificationIDs(batchID)
	s.Len(notificationIDs, 1)

	s.mockWebhook.SetResponse(http.StatusBadRequest, "bad request")

	notificationUUID, err := uuid.Parse(notificationIDs[0])
	s.Require().NoError(err)
	err = s.processTask(worker.TaskDeliverySMS, notificationUUID)
	s.Require().NoError(err)

	status := s.getNotificationStatus(notificationIDs[0])
	s.Equal("failed", status)

	resp2 := s.get(s.notificationURL + "/" + notificationIDs[0])
	defer resp2.Body.Close()
	s.Equal(http.StatusOK, resp2.StatusCode)
	n := s.decodeJSON(resp2)
	s.Equal("failed", n["status"])
	s.NotNil(n["error_message"])
}

func (s *IntegrationSuite) TestBatchDelivery_TemporaryFailureRetries() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905555555555", "channel": "email", "content": "Temp fail then success"},
		},
	}

	resp := s.post(s.batchURL, key, payload)
	defer resp.Body.Close()
	s.Equal(http.StatusAccepted, resp.StatusCode)
	result := s.decodeJSON(resp)
	batchID := result["batch_id"].(string)

	notificationIDs := s.getNotificationIDs(batchID)
	s.Len(notificationIDs, 1)
	notifID := notificationIDs[0]

	s.mockWebhook.SetResponse(http.StatusInternalServerError, "temporary error")

	notificationUUID, err := uuid.Parse(notifID)
	s.Require().NoError(err)
	_ = s.processTask(worker.TaskDeliveryEmail, notificationUUID)

	status := s.getNotificationStatus(notifID)
	s.Equal("pending", status)

	s.mockWebhook.SetResponse(http.StatusOK, `{"status":"delivered"}`)
	err = s.processTask(worker.TaskDeliveryEmail, notificationUUID)
	s.Require().NoError(err)

	status = s.getNotificationStatus(notifID)
	s.Equal("delivered", status)
}

func (s *IntegrationSuite) TestBatchDelivery_CancelAlreadyDeliveredFails() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905556666666", "channel": "push", "content": "Already delivered"},
		},
	}

	resp := s.post(s.batchURL, key, payload)
	defer resp.Body.Close()
	s.Equal(http.StatusAccepted, resp.StatusCode)
	result := s.decodeJSON(resp)
	batchID := result["batch_id"].(string)

	notificationIDs := s.getNotificationIDs(batchID)
	s.Len(notificationIDs, 1)

	s.mockWebhook.SetResponse(http.StatusOK, `{"status":"delivered"}`)
	notificationUUID, err := uuid.Parse(notificationIDs[0])
	s.Require().NoError(err)
	err = s.processTask(worker.TaskDeliveryPush, notificationUUID)
	s.Require().NoError(err)

	status := s.getNotificationStatus(notificationIDs[0])
	s.Equal("delivered", status)

	cancelResp := s.cancel(s.notificationURL + "/" + notificationIDs[0] + "/cancel")
	defer cancelResp.Body.Close()
	s.Equal(http.StatusBadRequest, cancelResp.StatusCode)
	cancelResult := s.decodeJSON(cancelResp)
	s.Contains(cancelResult["error"], "cannot be cancelled")
}

func (s *IntegrationSuite) TestBatchDelivery_CancelNonExistentNotification() {
	fakeID := uuid.New().String()
	cancelResp := s.cancel(s.notificationURL + "/" + fakeID + "/cancel")
	defer cancelResp.Body.Close()
	s.Equal(http.StatusNotFound, cancelResp.StatusCode)
}

func (s *IntegrationSuite) TestBatchDelivery_MultipleChannelsDeliveredIndividually() {
	key := newUUID()
	notifications := make([]map[string]string, 5)
	channels := []string{"sms", "email", "push", "sms", "email"}
	for i := range notifications {
		notifications[i] = map[string]string{
			"recipient":  fmt.Sprintf("user%d@test.com", i),
			"channel":    channels[i],
			"content":    fmt.Sprintf("Message %d", i),
			"priority":   "normal",
		}
	}
	payload := map[string]interface{}{"notifications": notifications}

	resp := s.post(s.batchURL, key, payload)
	defer resp.Body.Close()
	s.Equal(http.StatusAccepted, resp.StatusCode)
	result := s.decodeJSON(resp)
	batchID := result["batch_id"].(string)

	notificationIDs := s.getNotificationIDs(batchID)
	s.Len(notificationIDs, 5)

	taskTypes := []string{
		worker.TaskDeliverySMS,
		worker.TaskDeliveryEmail,
		worker.TaskDeliveryPush,
		worker.TaskDeliverySMS,
		worker.TaskDeliveryEmail,
	}

	for i, id := range notificationIDs {
		notificationUUID, err := uuid.Parse(id)
		s.Require().NoError(err)
		err = s.processTask(taskTypes[i], notificationUUID)
		s.Require().NoError(err)

		status := s.getNotificationStatus(id)
		s.Equal("delivered", status)
	}
}