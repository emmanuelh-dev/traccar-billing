package traccar

type userDTO struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type deviceDTO struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	UniqueID string `json:"uniqueId"`
	Status   string `json:"status"`
}

type serverDTO struct {
	Version string `json:"version"`
}
