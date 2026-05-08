package domain

type BatchStatus string

const (
	BatchStatusAccepted          BatchStatus = "accepted"
	BatchStatusProcessing        BatchStatus = "processing"
	BatchStatusCompleted         BatchStatus = "completed"
	BatchStatusPartiallyCompleted BatchStatus = "partially_completed"
)

type Channel string

const (
	ChannelSMS   Channel = "sms"
	ChannelEmail Channel = "email"
	ChannelPush  Channel = "push"
)

type Priority string

const (
	PriorityHigh   Priority = "high"
	PriorityNormal Priority = "normal"
	PriorityLow    Priority = "low"
)

type NotificationStatus string

const (
	NotificationStatusPending    NotificationStatus = "pending"
	NotificationStatusProcessing NotificationStatus = "processing"
	NotificationStatusDelivered  NotificationStatus = "delivered"
	NotificationStatusFailed     NotificationStatus = "failed"
	NotificationStatusCancelled  NotificationStatus = "cancelled"
)