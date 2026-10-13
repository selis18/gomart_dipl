package model

type RegisterAuthRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}
