package wallets

import "time"

const StatusActive = "ACTIVE"
const StatusInactive = "INACTIVE"

type Wallet struct {
	ID          string    `json:"id"`
	Chain       string    `json:"chain"`
	Address     string    `json:"address"`
	Label       string    `json:"label"`
	SigningType string    `json:"signing_type"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
