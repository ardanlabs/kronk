package dbsession

import (
	"time"

	"github.com/google/uuid"
)

type SessionData struct {
	SessionID     uuid.UUID `json:"session_id"`
	Status        Status    `json:"status"`
	Result        []byte    `json:"result"`
	StartedDate   time.Time `json:"started_date"`
	CompletedDate time.Time `json:"completed_date"`
}
