package transfers

type TransferJob struct {
	TransferID string `json:"transfer_id"`
	Attempt    int    `json:"attempt"`
}
