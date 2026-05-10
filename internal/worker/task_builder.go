package worker

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

func NewTaskSMS(notificationID uuid.UUID, correlationID string) (*asynq.Task, error) {
	return newDeliveryTask(TaskDeliverySMS, notificationID, correlationID)
}

func NewTaskEmail(notificationID uuid.UUID, correlationID string) (*asynq.Task, error) {
	return newDeliveryTask(TaskDeliveryEmail, notificationID, correlationID)
}

func NewTaskPush(notificationID uuid.UUID, correlationID string) (*asynq.Task, error) {
	return newDeliveryTask(TaskDeliveryPush, notificationID, correlationID)
}

func newDeliveryTask(taskType string, notificationID uuid.UUID, correlationID string) (*asynq.Task, error) {
	payload, err := json.Marshal(NotificationPayload{NotificationID: notificationID, CorrelationID: correlationID})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(taskType, payload), nil
}