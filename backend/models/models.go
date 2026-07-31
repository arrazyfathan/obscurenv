package models

type User struct {
	ID    string
	Email string
}

type AuthUser struct {
	ID string
}

type EnvVersion struct {
	ProjectSlug      string `json:"project_slug"`
	Environment      string `json:"environment"`
	Version          int    `json:"version"`
	EncryptedPayload string `json:"encrypted_payload"`
	Checksum         string `json:"checksum"`
}
