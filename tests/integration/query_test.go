//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	"github.com/samil/notification/internal/worker"
)

func (s *IntegrationSuite) TestListNotifications_Pagination() {
	for i := 0; i < 3; i++ {
		key := newUUID()
		notifications := make([]map[string]string, 5)
		for j := range notifications {
			notifications[j] = map[string]string{
				"recipient": fmt.Sprintf("user%d_%d@test.com", i, j),
				"channel":   "email",
				"content":   fmt.Sprintf("Pagination test %d-%d", i, j),
			}
		}
		payload := map[string]interface{}{"notifications": notifications}
		resp := s.post(s.batchURL, key, payload)
		defer resp.Body.Close()
		s.Equal(http.StatusAccepted, resp.StatusCode)
	}

	u, _ := url.Parse(s.notificationURL)
	q := u.Query()
	q.Set("page", "1")
	q.Set("limit", "5")
	u.RawQuery = q.Encode()

	resp := s.get(u.String())
	defer resp.Body.Close()
	s.Equal(http.StatusOK, resp.StatusCode)

	var page1 paginatedResult
	s.Require().NoError(json.NewDecoder(resp.Body).Decode(&page1))
	s.Len(page1.Data, 5)
	s.Equal(15, page1.Meta.Total)
	s.Equal(1, page1.Meta.Page)
	s.Equal(5, page1.Meta.Limit)

	u2, _ := url.Parse(s.notificationURL)
	q2 := u2.Query()
	q2.Set("page", "2")
	q2.Set("limit", "5")
	u2.RawQuery = q2.Encode()

	resp2 := s.get(u2.String())
	defer resp2.Body.Close()
	s.Equal(http.StatusOK, resp2.StatusCode)

	var page2 paginatedResult
	s.Require().NoError(json.NewDecoder(resp2.Body).Decode(&page2))
	s.Len(page2.Data, 5)
	s.Equal(15, page2.Meta.Total)
	s.Equal(2, page2.Meta.Page)

	page1IDs := make(map[string]bool)
	for _, n := range page1.Data {
		page1IDs[n["id"].(string)] = true
	}
	for _, n := range page2.Data {
		s.False(page1IDs[n["id"].(string)], "page 2 should not overlap with page 1")
	}

	u3, _ := url.Parse(s.notificationURL)
	q3 := u3.Query()
	q3.Set("page", "4")
	q3.Set("limit", "5")
	u3.RawQuery = q3.Encode()

	resp3 := s.get(u3.String())
	defer resp3.Body.Close()
	s.Equal(http.StatusOK, resp3.StatusCode)

	var page4 paginatedResult
	s.Require().NoError(json.NewDecoder(resp3.Body).Decode(&page4))
	s.Empty(page4.Data)
	s.Equal(15, page4.Meta.Total)
}

func (s *IntegrationSuite) TestListNotifications_FilterByStatus() {
	key1 := newUUID()
	payload1 := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551111111", "channel": "sms", "content": "Will deliver"},
			{"recipient": "+905551111112", "channel": "sms", "content": "Will stay pending"},
		},
	}
	resp := s.post(s.batchURL, key1, payload1)
	defer resp.Body.Close()
	s.Equal(http.StatusAccepted, resp.StatusCode)
	result := s.decodeJSON(resp)
	batchID := result["batch_id"].(string)

	notificationIDs := s.getNotificationIDs(batchID)
	s.Len(notificationIDs, 2)

	deliverUUID, err := uuid.Parse(notificationIDs[0])
	s.Require().NoError(err)
	err = s.processTask(worker.TaskDeliverySMS, deliverUUID)
	s.Require().NoError(err)

	s.Equal("delivered", s.getNotificationStatus(notificationIDs[0]))
	s.Equal("pending", s.getNotificationStatus(notificationIDs[1]))

	u, _ := url.Parse(s.notificationURL)
	q := u.Query()
	q.Set("status", "delivered")
	q.Set("limit", "10")
	u.RawQuery = q.Encode()

	resp2 := s.get(u.String())
	defer resp2.Body.Close()
	s.Equal(http.StatusOK, resp2.StatusCode)

	var delivered paginatedResult
	s.Require().NoError(json.NewDecoder(resp2.Body).Decode(&delivered))
	s.Equal(1, delivered.Meta.Total)
	s.Equal("delivered", delivered.Data[0]["status"])
}

func (s *IntegrationSuite) TestListNotifications_FilterByChannel() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551111111", "channel": "sms", "content": "SMS note"},
			{"recipient": "user@test.com", "channel": "email", "content": "Email note"},
			{"recipient": "device123", "channel": "push", "content": "Push note"},
		},
	}
	resp := s.post(s.batchURL, key, payload)
	defer resp.Body.Close()
	s.Equal(http.StatusAccepted, resp.StatusCode)

	u, _ := url.Parse(s.notificationURL)
	q := u.Query()
	q.Set("channel", "email")
	q.Set("limit", "10")
	u.RawQuery = q.Encode()

	resp2 := s.get(u.String())
	defer resp2.Body.Close()
	s.Equal(http.StatusOK, resp2.StatusCode)

	var emailOnly paginatedResult
	s.Require().NoError(json.NewDecoder(resp2.Body).Decode(&emailOnly))
	s.Equal(1, emailOnly.Meta.Total)
	s.Equal("email", emailOnly.Data[0]["channel"])
}

func (s *IntegrationSuite) TestGetNotificationByID() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905559999999", "channel": "sms", "content": "Detailed notification", "priority": "high"},
		},
	}
	resp := s.post(s.batchURL, key, payload)
	defer resp.Body.Close()
	s.Equal(http.StatusAccepted, resp.StatusCode)
	result := s.decodeJSON(resp)
	batchID := result["batch_id"].(string)

	notificationIDs := s.getNotificationIDs(batchID)
	s.Len(notificationIDs, 1)

	resp2 := s.get(s.notificationURL + "/" + notificationIDs[0])
	defer resp2.Body.Close()
	s.Equal(http.StatusOK, resp2.StatusCode)

	n := s.decodeJSON(resp2)
	s.Equal(notificationIDs[0], n["id"])
	s.Equal(batchID, n["batch_id"])
	s.Equal("+905559999999", n["recipient"])
	s.Equal("sms", n["channel"])
	s.Equal("Detailed notification", n["content"])
	s.Equal("high", n["priority"])
	s.Equal("pending", n["status"])
	s.Nil(n["error_message"])
	s.Equal(float64(0), n["retry_count"])
}

func (s *IntegrationSuite) TestGetNotificationByID_NotFound() {
	fakeID := uuid.New().String()
	resp := s.get(s.notificationURL + "/" + fakeID)
	defer resp.Body.Close()
	s.Equal(http.StatusNotFound, resp.StatusCode)
}

func (s *IntegrationSuite) TestGetNotificationByID_InvalidUUID() {
	resp := s.get(s.notificationURL + "/not-a-uuid")
	defer resp.Body.Close()
	s.Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *IntegrationSuite) TestGetBatchByID() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905557777777", "channel": "email", "content": "Batch query test"},
		},
	}
	resp := s.post(s.batchURL, key, payload)
	defer resp.Body.Close()
	s.Equal(http.StatusAccepted, resp.StatusCode)
	result := s.decodeJSON(resp)
	batchID := result["batch_id"].(string)

	resp2 := s.get(s.batchesURL + "/" + batchID)
	defer resp2.Body.Close()
	s.Equal(http.StatusOK, resp2.StatusCode)

	batch := s.decodeJSON(resp2)
	s.Equal(batchID, batch["batch_id"])
	s.Equal(key, batch["idempotency_key"])
	s.Equal("accepted", batch["status"])
	s.Equal(float64(1), batch["total_count"])
}

func (s *IntegrationSuite) TestGetBatchByID_NotFound() {
	fakeID := uuid.New().String()
	resp := s.get(s.batchesURL + "/" + fakeID)
	defer resp.Body.Close()
	s.Equal(http.StatusNotFound, resp.StatusCode)
}

func (s *IntegrationSuite) TestListNotifications_FilterByBatchID() {
	key1 := newUUID()
	key2 := newUUID()

	payload1 := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "batch1_user@test.com", "channel": "email", "content": "Batch 1 note 1"},
			{"recipient": "+905550000001", "channel": "sms", "content": "Batch 1 note 2"},
		},
	}
	resp1 := s.post(s.batchURL, key1, payload1)
	defer resp1.Body.Close()
	s.Equal(http.StatusAccepted, resp1.StatusCode)
	result1 := s.decodeJSON(resp1)
	batchID1 := result1["batch_id"].(string)

	payload2 := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "batch2_user@test.com", "channel": "email", "content": "Batch 2 note 1"},
		},
	}
	resp2 := s.post(s.batchURL, key2, payload2)
	defer resp2.Body.Close()
	s.Equal(http.StatusAccepted, resp2.StatusCode)

	u, _ := url.Parse(s.notificationURL)
	q := u.Query()
	q.Set("batch_id", batchID1)
	q.Set("limit", "10")
	u.RawQuery = q.Encode()

	resp3 := s.get(u.String())
	defer resp3.Body.Close()
	s.Equal(http.StatusOK, resp3.StatusCode)

	var batch1Only paginatedResult
	s.Require().NoError(json.NewDecoder(resp3.Body).Decode(&batch1Only))
	s.Equal(2, batch1Only.Meta.Total)
	for _, n := range batch1Only.Data {
		s.Equal(batchID1, n["batch_id"])
	}
}

func (s *IntegrationSuite) TestListNotifications_InvalidParams() {
	u, _ := url.Parse(s.notificationURL)
	q := u.Query()
	q.Set("page", "0")
	q.Set("limit", "10")
	u.RawQuery = q.Encode()

	resp := s.get(u.String())
	defer resp.Body.Close()
	s.Equal(http.StatusBadRequest, resp.StatusCode)

	u2, _ := url.Parse(s.notificationURL)
	q2 := u2.Query()
	q2.Set("limit", "500")
	u2.RawQuery = q2.Encode()

	resp2 := s.get(u2.String())
	defer resp2.Body.Close()
	s.Equal(http.StatusBadRequest, resp2.StatusCode)

	u3, _ := url.Parse(s.notificationURL)
	q3 := u3.Query()
	q3.Set("status", "invalid_status")
	u3.RawQuery = q3.Encode()

	resp3 := s.get(u3.String())
	defer resp3.Body.Close()
	s.Equal(http.StatusBadRequest, resp3.StatusCode)
}

type paginationMeta struct {
	Total int `json:"total"`
	Page  int `json:"page"`
	Limit int `json:"limit"`
}

type paginatedResult struct {
	Data []map[string]interface{} `json:"data"`
	Meta paginationMeta            `json:"meta"`
}