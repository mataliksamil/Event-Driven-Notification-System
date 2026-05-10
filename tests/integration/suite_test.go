//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/samil/notification/internal/adapter/batch"
	notificationAdapter "github.com/samil/notification/internal/adapter/notification"
	"github.com/samil/notification/internal/adapter/middleware"
	"github.com/samil/notification/internal/config"
	"github.com/samil/notification/internal/delivery"
	"github.com/samil/notification/internal/producer"
	redisSvc "github.com/samil/notification/internal/redis"
	"github.com/samil/notification/internal/service"
	"github.com/samil/notification/internal/storage"
	"github.com/samil/notification/internal/worker"
	"github.com/stretchr/testify/suite"
)

type IntegrationSuite struct {
	suite.Suite
	server        *httptest.Server
	mockWebhook   *mockWebhookServer
	pool          *pgxpool.Pool
	redisClient   *redis.Client
	asynqClient   *asynq.Client
	inspector     *asynq.Inspector
	processor     *worker.NotificationProcessor
	repo          *storage.PostgresRepository
	batchURL      string
	notificationURL string
	batchesURL    string
}

type mockWebhookServer struct {
	server   *httptest.Server
	mu       sync.RWMutex
	statusCode int
	body       string
}

func (m *mockWebhookServer) SetResponse(code int, body string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statusCode = code
	m.body = body
}

func (m *mockWebhookServer) GetResponse() (int, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.statusCode, m.body
}

func (m *mockWebhookServer) URL() string {
	return m.server.URL
}

func newMockWebhookServer() *mockWebhookServer {
	m := &mockWebhookServer{statusCode: http.StatusOK, body: `{"status":"delivered"}`}
	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/__config" {
			if r.Method == http.MethodPost {
				var cfg struct {
					StatusCode int    `json:"status_code"`
					Body        string `json:"body"`
				}
				if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				m.SetResponse(cfg.StatusCode, cfg.Body)
				w.WriteHeader(http.StatusOK)
				return
			}
			code, body := m.GetResponse()
			json.NewEncoder(w).Encode(map[string]interface{}{"status_code": code, "body": body})
			return
		}
		code, body := m.GetResponse()
		w.WriteHeader(code)
		if body != "" {
			w.Write([]byte(body))
		} else {
			w.Write([]byte(http.StatusText(code)))
		}
	}))
	return m
}

func (s *IntegrationSuite) SetupSuite() {
	cfg, err := config.Load()
	s.Require().NoError(err)

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, cfg.DSN())
	s.Require().NoError(err)
	s.pool = pool
	suitePool = pool

	redisClient := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%d", cfg.RedisHost, cfg.RedisPort),
	})
	s.Require().NoError(redisClient.Ping(ctx).Err())
	s.redisClient = redisClient

	s.mockWebhook = newMockWebhookServer()

	s.repo = storage.NewPostgresRepository(pool)
	idempotencySvc := redisSvc.NewIdempotencyService(redisClient)
	s.asynqClient = producer.NewClient(cfg)
	prod := producer.NewProducer(s.asynqClient)
	batchSvc := service.NewBatchService(s.repo, prod)
	notificationSvc := service.NewNotificationService(s.repo)

	webhookClient := delivery.NewWebhookClient(s.mockWebhook.URL())
	s.processor = worker.NewNotificationProcessor(s.repo, webhookClient)

	batchHandler := batch.NewHandler(batchSvc, notificationSvc)
	notificationHandler := notificationAdapter.NewHandler(notificationSvc)
	idempotencyMW := middleware.NewIdempotency(idempotencySvc)
	correlationMW := middleware.NewCorrelation()

	inspector := asynq.NewInspector(asynq.RedisClientOpt{
		Addr: fmt.Sprintf("%s:%d", cfg.RedisHost, cfg.RedisPort),
	})
	s.inspector = inspector

	r := chi.NewRouter()
	r.Use(correlationMW.Handler)
	r.Route("/api/v1", func(r chi.Router) {
		r.With(idempotencyMW.Handle).Post("/notifications/batches", batchHandler.CreateBatch)
		r.Get("/batches/{batchId}", batchHandler.GetBatch)
		r.Mount("/notifications", notificationHandler.Routes())
	})

	s.server = httptest.NewServer(r)
	s.batchURL = s.server.URL + "/api/v1/notifications/batches"
	s.notificationURL = s.server.URL + "/api/v1/notifications"
	s.batchesURL = s.server.URL + "/api/v1/batches"
}

func (s *IntegrationSuite) TearDownSuite() {
	s.server.Close()
	s.mockWebhook.server.Close()
	s.pool.Close()
	s.redisClient.Close()
	s.asynqClient.Close()
	s.inspector.Close()
}

func (s *IntegrationSuite) SetupTest() {
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

	s.mockWebhook.SetResponse(http.StatusOK, `{"status":"delivered"}`)
}

func (s *IntegrationSuite) post(url string, key string, body interface{}) *http.Response {
	var buf bytes.Buffer
	if body != nil {
		err := json.NewEncoder(&buf).Encode(body)
		s.Require().NoError(err)
	}

	req, err := http.NewRequest(http.MethodPost, url, &buf)
	s.Require().NoError(err)
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}

	resp, err := http.DefaultClient.Do(req)
	s.Require().NoError(err)
	return resp
}

func (s *IntegrationSuite) get(url string) *http.Response {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	s.Require().NoError(err)
	resp, err := http.DefaultClient.Do(req)
	s.Require().NoError(err)
	return resp
}

func (s *IntegrationSuite) cancel(url string) *http.Response {
	req, err := http.NewRequest(http.MethodPost, url, nil)
	s.Require().NoError(err)
	resp, err := http.DefaultClient.Do(req)
	s.Require().NoError(err)
	return resp
}

func (s *IntegrationSuite) decodeJSON(resp *http.Response) map[string]interface{} {
	defer resp.Body.Close()
	var result map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&result)
	s.Require().NoError(err)
	return result
}

func (s *IntegrationSuite) decodeJSONArray(resp *http.Response) []map[string]interface{} {
	defer resp.Body.Close()
	var result struct {
		Data []map[string]interface{} `json:"data"`
		Meta struct {
			Total float64 `json:"total"`
			Page  float64 `json:"page"`
			Limit float64 `json:"limit"`
		} `json:"meta"`
	}
	err := json.NewDecoder(resp.Body).Decode(&result)
	s.Require().NoError(err)
	return result.Data
}

func (s *IntegrationSuite) decodeJSONWithMeta(resp *http.Response) ([]map[string]interface{}, map[string]interface{}) {
	defer resp.Body.Close()
	var result struct {
		Data []map[string]interface{} `json:"data"`
		Meta map[string]interface{}    `json:"meta"`
	}
	err := json.NewDecoder(resp.Body).Decode(&result)
	s.Require().NoError(err)
	return result.Data, result.Meta
}

func (s *IntegrationSuite) pendingTasks(queue string) []*asynq.TaskInfo {
	tasks, err := s.inspector.ListPendingTasks(queue)
	if err != nil {
		if err == asynq.ErrQueueNotFound || err.Error() == "asynq: queue not found" {
			return nil
		}
		s.Require().NoError(err)
	}
	return tasks
}

func (s *IntegrationSuite) drainQueue(queue string) []*asynq.TaskInfo {
	tasks := s.pendingTasks(queue)
	_, _ = s.inspector.DeleteAllPendingTasks(queue)
	return tasks
}

func (s *IntegrationSuite) drainAllQueues() {
	for _, queue := range []string{"default", "critical", "low"} {
		s.drainQueue(queue)
	}
}

func (s *IntegrationSuite) processTask(taskType string, notificationID uuid.UUID) error {
	payload, err := json.Marshal(worker.NotificationPayload{NotificationID: notificationID})
	s.Require().NoError(err)
	task := asynq.NewTask(taskType, payload)
	return s.processor.ProcessTask(context.Background(), task)
}

func (s *IntegrationSuite) createBatch(key string, notifications []map[string]string) map[string]interface{} {
	payload := map[string]interface{}{"notifications": notifications}
	resp := s.post(s.batchURL, key, payload)
	defer resp.Body.Close()
	s.Equal(http.StatusAccepted, resp.StatusCode)
	return s.decodeJSON(resp)
}

func (s *IntegrationSuite) getNotificationIDs(batchID string) []string {
	rows, err := s.pool.Query(context.Background(),
		"SELECT id FROM notifications WHERE batch_id = $1 ORDER BY created_at", batchID)
	s.Require().NoError(err)
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		s.Require().NoError(rows.Scan(&id))
		ids = append(ids, id)
	}
	return ids
}

func (s *IntegrationSuite) getNotificationStatus(id string) string {
	var status string
	err := s.pool.QueryRow(context.Background(),
		"SELECT status FROM notifications WHERE id = $1", id).Scan(&status)
	s.Require().NoError(err)
	return status
}

func (s *IntegrationSuite) readBody(resp *http.Response) []byte {
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	s.Require().NoError(err)
	return b
}

func newUUID() string {
	return uuid.New().String()
}

func ctx() context.Context {
	return context.Background()
}

func queryRow(c context.Context, query string, dest ...interface{}) error {
	return suitePool.QueryRow(c, query).Scan(dest...)
}

var suitePool *pgxpool.Pool

func jsonUnmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

func TestIntegrationSuite(t *testing.T) {
	suite.Run(t, new(IntegrationSuite))
}