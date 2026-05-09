//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/samil/notification/internal/adapter/batch"
	"github.com/samil/notification/internal/adapter/middleware"
	"github.com/samil/notification/internal/config"
	"github.com/samil/notification/internal/producer"
	redisSvc "github.com/samil/notification/internal/redis"
	"github.com/samil/notification/internal/service"
	"github.com/samil/notification/internal/storage"
	"github.com/samil/notification/internal/worker"
	"github.com/stretchr/testify/suite"
)

type BatchTestSuite struct {
	suite.Suite
	server      *httptest.Server
	pool        *pgxpool.Pool
	redisClient *redis.Client
	asynqClient *asynq.Client
	inspector   *asynq.Inspector
	baseURL     string
}

func (s *BatchTestSuite) SetupSuite() {
	cfg, err := config.Load()
	s.Require().NoError(err)

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, cfg.DSN())
	s.Require().NoError(err)
	s.pool = pool

	redisClient := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%d", cfg.RedisHost, cfg.RedisPort),
	})
	s.Require().NoError(redisClient.Ping(ctx).Err())
	s.redisClient = redisClient

	repo := storage.NewPostgresRepository(pool)
	idempotencySvc := redisSvc.NewIdempotencyService(redisClient)
	asynqClient := producer.NewClient(cfg)
	s.asynqClient = asynqClient
	prod := producer.NewProducer(asynqClient)
	batchSvc := service.NewBatchService(repo, prod)
	batchHandler := batch.NewHandler(batchSvc)
	idempotencyMW := middleware.NewIdempotency(idempotencySvc)

	inspector := asynq.NewInspector(asynq.RedisClientOpt{
		Addr: fmt.Sprintf("%s:%d", cfg.RedisHost, cfg.RedisPort),
	})
	s.inspector = inspector

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.With(idempotencyMW.Handle).Mount("/notifications/batches", batchHandler.Routes())
	})

	s.server = httptest.NewServer(r)
	s.baseURL = s.server.URL + "/api/v1/notifications/batches"
}

func (s *BatchTestSuite) TearDownSuite() {
	s.server.Close()
	s.pool.Close()
	s.redisClient.Close()
	s.asynqClient.Close()
	s.inspector.Close()
}

func (s *BatchTestSuite) SetupTest() {
	_, err := s.pool.Exec(context.Background(), "TRUNCATE notifications, batches CASCADE")
	s.Require().NoError(err)

	ctx := context.Background()
	keys, err := s.redisClient.Keys(ctx, "idempotency:*").Result()
	s.Require().NoError(err)
	if len(keys) > 0 {
		s.Require().NoError(s.redisClient.Del(ctx, keys...).Err())
	}

	for _, queue := range []string{"default", "critical", "low"} {
		_, _ = s.inspector.DeleteAllPendingTasks(queue)
	}
}

func (s *BatchTestSuite) post(key string, body interface{}) *http.Response {
	var buf bytes.Buffer
	if body != nil {
		err := json.NewEncoder(&buf).Encode(body)
		s.Require().NoError(err)
	}

	req, err := http.NewRequest(http.MethodPost, s.baseURL, &buf)
	s.Require().NoError(err)
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}

	resp, err := http.DefaultClient.Do(req)
	s.Require().NoError(err)
	return resp
}

func (s *BatchTestSuite) decodeJSON(resp *http.Response) map[string]interface{} {
	defer resp.Body.Close()
	var result map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&result)
	s.Require().NoError(err)
	return result
}

func (s *BatchTestSuite) pendingTasks(queue string) []*asynq.TaskInfo {
	tasks, err := s.inspector.ListPendingTasks(queue)
	if err != nil {
		if err == asynq.ErrQueueNotFound || err.Error() == "asynq: queue not found" {
			return nil
		}
		s.Require().NoError(err)
	}
	return tasks
}

func (s *BatchTestSuite) TestSuccess() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551234567", "channel": "sms", "content": "Hello"},
			{"recipient": "user@example.com", "channel": "email", "content": "World", "priority": "high"},
		},
	}

	resp := s.post(key, payload)
	defer resp.Body.Close()

	s.Equal(http.StatusAccepted, resp.StatusCode)
	result := s.decodeJSON(resp)
	s.NotEmpty(result["batch_id"])
	s.Equal("accepted", result["status"])
	s.Equal(float64(2), result["total_count"])
	s.NotEmpty(result["accepted_at"])
}

func (s *BatchTestSuite) TestMissingIdempotencyKey() {
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551234567", "channel": "sms", "content": "Hello"},
		},
	}

	resp := s.post("", payload)
	defer resp.Body.Close()

	s.Equal(http.StatusBadRequest, resp.StatusCode)
	result := s.decodeJSON(resp)
	s.Contains(result["error"], "Idempotency-Key")
}

func (s *BatchTestSuite) TestInvalidIdempotencyKey() {
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551234567", "channel": "sms", "content": "Hello"},
		},
	}

	resp := s.post("not-a-uuid", payload)
	defer resp.Body.Close()

	s.Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *BatchTestSuite) TestEmptyNotifications() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{},
	}

	resp := s.post(key, payload)
	defer resp.Body.Close()

	s.Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *BatchTestSuite) TestExceedsMaxNotifications() {
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

	resp := s.post(key, payload)
	defer resp.Body.Close()

	s.Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *BatchTestSuite) TestInvalidChannel() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551234567", "channel": "fax", "content": "Hello"},
		},
	}

	resp := s.post(key, payload)
	defer resp.Body.Close()

	s.Equal(http.StatusBadRequest, resp.StatusCode)
	result := s.decodeJSON(resp)
	s.Contains(result["error"], "invalid channel")
}

func (s *BatchTestSuite) TestInvalidPriority() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551234567", "channel": "sms", "content": "Hello", "priority": "urgent"},
		},
	}

	resp := s.post(key, payload)
	defer resp.Body.Close()

	s.Equal(http.StatusBadRequest, resp.StatusCode)
	result := s.decodeJSON(resp)
	s.Contains(result["error"], "invalid priority")
}

func (s *BatchTestSuite) TestMissingRequiredFields() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"channel": "sms", "content": "Hello"},
		},
	}

	resp := s.post(key, payload)
	defer resp.Body.Close()

	s.Equal(http.StatusBadRequest, resp.StatusCode)
	result := s.decodeJSON(resp)
	s.Contains(result["error"], "required")
}

func (s *BatchTestSuite) TestDuplicateIdempotencyKey_ReturnsCachedResponse() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551234567", "channel": "sms", "content": "Hello"},
		},
	}

	resp1 := s.post(key, payload)
	defer resp1.Body.Close()
	s.Equal(http.StatusAccepted, resp1.StatusCode)
	result1 := s.decodeJSON(resp1)
	batchID := result1["batch_id"]

	resp2 := s.post(key, payload)
	defer resp2.Body.Close()
	s.Equal(http.StatusAccepted, resp2.StatusCode)
	result2 := s.decodeJSON(resp2)
	s.Equal(batchID, result2["batch_id"])
	s.Equal("accepted", result2["status"])
}

func (s *BatchTestSuite) TestProcessingStatus() {
	key := newUUID()

	err := s.redisClient.Set(context.Background(), "idempotency:"+key, "processing", 0).Err()
	s.Require().NoError(err)

	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551234567", "channel": "sms", "content": "Hello"},
		},
	}

	resp := s.post(key, payload)
	defer resp.Body.Close()

	s.Equal(http.StatusAccepted, resp.StatusCode)
	result := s.decodeJSON(resp)
	s.Equal("processing", result["status"])
}

func (s *BatchTestSuite) TestErrorReleasesIdempotencyKey() {
	key := newUUID()

	badPayload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551234567", "channel": "fax", "content": "Hello"},
		},
	}

	resp := s.post(key, badPayload)
	defer resp.Body.Close()
	s.Equal(http.StatusBadRequest, resp.StatusCode)

	validPayload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551234567", "channel": "sms", "content": "Hello"},
		},
	}

	resp2 := s.post(key, validPayload)
	defer resp2.Body.Close()
	s.Equal(http.StatusAccepted, resp2.StatusCode)
}

func (s *BatchTestSuite) TestDefaultPriorityIsNormal() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551234567", "channel": "sms", "content": "Hello"},
		},
	}

	resp := s.post(key, payload)
	defer resp.Body.Close()
	s.Equal(http.StatusAccepted, resp.StatusCode)

	var priority string
	err := s.pool.QueryRow(context.Background(),
		"SELECT priority FROM notifications LIMIT 1",
	).Scan(&priority)
	s.Require().NoError(err)
	s.Equal("normal", priority)
}

func (s *BatchTestSuite) TestEnqueue_SmsTaskInDefaultQueue() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551234567", "channel": "sms", "content": "Hello"},
		},
	}

	resp := s.post(key, payload)
	defer resp.Body.Close()
	s.Equal(http.StatusAccepted, resp.StatusCode)

	tasks := s.pendingTasks("default")
	s.Len(tasks, 1)
	s.Equal(worker.TaskDeliverySMS, tasks[0].Type)

	var p worker.NotificationPayload
	s.Require().NoError(json.Unmarshal(tasks[0].Payload, &p))
	s.NotEmpty(p.NotificationID)
}

func (s *BatchTestSuite) TestEnqueue_EmailTaskInDefaultQueue() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "user@example.com", "channel": "email", "content": "Hello"},
		},
	}

	resp := s.post(key, payload)
	defer resp.Body.Close()
	s.Equal(http.StatusAccepted, resp.StatusCode)

	tasks := s.pendingTasks("default")
	s.Len(tasks, 1)
	s.Equal(worker.TaskDeliveryEmail, tasks[0].Type)
}

func (s *BatchTestSuite) TestEnqueue_PushTaskInDefaultQueue() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "device123", "channel": "push", "content": "Hello"},
		},
	}

	resp := s.post(key, payload)
	defer resp.Body.Close()
	s.Equal(http.StatusAccepted, resp.StatusCode)

	tasks := s.pendingTasks("default")
	s.Len(tasks, 1)
	s.Equal(worker.TaskDeliveryPush, tasks[0].Type)
}

func (s *BatchTestSuite) TestEnqueue_HighPriorityInCriticalQueue() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551234567", "channel": "sms", "content": "Urgent", "priority": "high"},
		},
	}

	resp := s.post(key, payload)
	defer resp.Body.Close()
	s.Equal(http.StatusAccepted, resp.StatusCode)

	critical := s.pendingTasks("critical")
	s.Len(critical, 1)
	s.Equal(worker.TaskDeliverySMS, critical[0].Type)
}

func (s *BatchTestSuite) TestEnqueue_LowPriorityInLowQueue() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551234567", "channel": "sms", "content": "Low prio", "priority": "low"},
		},
	}

	resp := s.post(key, payload)
	defer resp.Body.Close()
	s.Equal(http.StatusAccepted, resp.StatusCode)

	lowTasks := s.pendingTasks("low")
	s.Len(lowTasks, 1)
	s.Equal(worker.TaskDeliverySMS, lowTasks[0].Type)
}

func (s *BatchTestSuite) TestEnqueue_NotificationPayloadContainsID() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551234567", "channel": "sms", "content": "Hello"},
		},
	}

	resp := s.post(key, payload)
	defer resp.Body.Close()
	s.Equal(http.StatusAccepted, resp.StatusCode)

	var notificationID string
	err := s.pool.QueryRow(context.Background(),
		"SELECT id FROM notifications LIMIT 1",
	).Scan(&notificationID)
	s.Require().NoError(err)

	tasks := s.pendingTasks("default")
	s.Len(tasks, 1)

	var p worker.NotificationPayload
	s.Require().NoError(json.Unmarshal(tasks[0].Payload, &p))
	s.Equal(notificationID, p.NotificationID.String())
}

func (s *BatchTestSuite) TestEnqueue_MultipleNotificationsMultipleQueues() {
	key := newUUID()
	payload := map[string]interface{}{
		"notifications": []map[string]string{
			{"recipient": "+905551234567", "channel": "sms", "content": "Urgent", "priority": "high"},
			{"recipient": "user@example.com", "channel": "email", "content": "Normal"},
			{"recipient": "device123", "channel": "push", "content": "Low prio", "priority": "low"},
		},
	}

	resp := s.post(key, payload)
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

func TestIntegration(t *testing.T) {
	s := &BatchTestSuite{}

	s.SetT(t)
	s.SetupSuite()
	defer s.TearDownSuite()

	cases := []struct {
		name string
		fn   func(*BatchTestSuite)
	}{
		{"Success", (*BatchTestSuite).TestSuccess},
		{"MissingIdempotencyKey", (*BatchTestSuite).TestMissingIdempotencyKey},
		{"InvalidIdempotencyKey", (*BatchTestSuite).TestInvalidIdempotencyKey},
		{"EmptyNotifications", (*BatchTestSuite).TestEmptyNotifications},
		{"ExceedsMaxNotifications", (*BatchTestSuite).TestExceedsMaxNotifications},
		{"InvalidChannel", (*BatchTestSuite).TestInvalidChannel},
		{"InvalidPriority", (*BatchTestSuite).TestInvalidPriority},
		{"MissingRequiredFields", (*BatchTestSuite).TestMissingRequiredFields},
		{"DuplicateIdempotencyKey_ReturnsCachedResponse", (*BatchTestSuite).TestDuplicateIdempotencyKey_ReturnsCachedResponse},
		{"ProcessingStatus", (*BatchTestSuite).TestProcessingStatus},
		{"ErrorReleasesIdempotencyKey", (*BatchTestSuite).TestErrorReleasesIdempotencyKey},
		{"DefaultPriorityIsNormal", (*BatchTestSuite).TestDefaultPriorityIsNormal},
		{"Enqueue_SmsTaskInDefaultQueue", (*BatchTestSuite).TestEnqueue_SmsTaskInDefaultQueue},
		{"Enqueue_EmailTaskInDefaultQueue", (*BatchTestSuite).TestEnqueue_EmailTaskInDefaultQueue},
		{"Enqueue_PushTaskInDefaultQueue", (*BatchTestSuite).TestEnqueue_PushTaskInDefaultQueue},
		{"Enqueue_HighPriorityInCriticalQueue", (*BatchTestSuite).TestEnqueue_HighPriorityInCriticalQueue},
		{"Enqueue_LowPriorityInLowQueue", (*BatchTestSuite).TestEnqueue_LowPriorityInLowQueue},
		{"Enqueue_NotificationPayloadContainsID", (*BatchTestSuite).TestEnqueue_NotificationPayloadContainsID},
		{"Enqueue_MultipleNotificationsMultipleQueues", (*BatchTestSuite).TestEnqueue_MultipleNotificationsMultipleQueues},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s.SetT(t)
			s.SetupTest()
			tc.fn(s)
		})
	}
}

func newUUID() string {
	return uuid.New().String()
}