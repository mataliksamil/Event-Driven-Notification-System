package domain

type BatchStatus string

const (
	BatchStatusAccepted            BatchStatus = "accepted"
	BatchStatusProcessing          BatchStatus = "processing"
	BatchStatusCompleted           BatchStatus = "completed"
	BatchStatusPartiallyCompleted  BatchStatus = "partially_completed"
)

func (s BatchStatus) IsValid() bool {
	switch s {
	case BatchStatusAccepted, BatchStatusProcessing, BatchStatusCompleted, BatchStatusPartiallyCompleted:
		return true
	}
	return false
}

type Channel string

const (
	ChannelSMS   Channel = "sms"
	ChannelEmail Channel = "email"
	ChannelPush  Channel = "push"
)

func (c Channel) IsValid() bool {
	switch c {
	case ChannelSMS, ChannelEmail, ChannelPush:
		return true
	}
	return false
}

type Priority string

const (
	PriorityHigh   Priority = "high"
	PriorityNormal Priority = "normal"
	PriorityLow    Priority = "low"
)

func (p Priority) IsValid() bool {
	switch p {
	case PriorityHigh, PriorityNormal, PriorityLow:
		return true
	}
	return false
}

type NotificationStatus string

const (
	NotificationStatusPending    NotificationStatus = "pending"
	NotificationStatusProcessing  NotificationStatus = "processing"
	NotificationStatusDelivered   NotificationStatus = "delivered"
	NotificationStatusFailed     NotificationStatus = "failed"
	NotificationStatusCancelled  NotificationStatus = "cancelled"
)

func (s NotificationStatus) IsValid() bool {
	switch s {
	case NotificationStatusPending, NotificationStatusProcessing, NotificationStatusDelivered, NotificationStatusFailed, NotificationStatusCancelled:
		return true
	}
	return false
}