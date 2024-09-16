package database

type User struct {
	ID       int    `json:"id,omitempty"`
	Username string `json:"username,omitempty"`
}
