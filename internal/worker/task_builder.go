package worker

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

func NewTaskSMS(notificationID uuid.UUID) (*asynq.Task, error) {
	return newDeliveryTask(TaskDeliverySMS, notificationID)
}

func NewTaskEmail(notificationID uuid.UUID) (*asynq.Task, error) {
	return newDeliveryTask(TaskDeliveryEmail, notificationID)
}

func NewTaskPush(notificationID uuid.UUID) (*asynq.Task, error) {
	return newDeliveryTask(TaskDeliveryPush, notificationID)
}

func newDeliveryTask(taskType string, notificationID uuid.UUID) (*asynq.Task, error) {
	payload, err := json.Marshal(NotificationPayload{NotificationID: notificationID})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(taskType, payload), nil
}