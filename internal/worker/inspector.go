package worker

import (
	"github.com/hibiken/asynq"
	"github.com/samil/notification/internal/metrics"
)

type AsynqInspector struct {
	inspector *asynq.Inspector
}

func NewAsynqInspector(redisOpt asynq.RedisClientOpt) *AsynqInspector {
	return &AsynqInspector{
		inspector: asynq.NewInspector(redisOpt),
	}
}

func (a *AsynqInspector) GetQueueInfo(queue string) (*metrics.QueueInfo, error) {
	info, err := a.inspector.GetQueueInfo(queue)
	if err != nil {
		return nil, err
	}
	return &metrics.QueueInfo{
		Size:    info.Size,
		Active:  info.Active,
		Pending: info.Pending,
		Retry:   info.Retry,
	}, nil
}

func (a *AsynqInspector) Close() error {
	return a.inspector.Close()
}