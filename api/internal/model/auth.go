package model

type LoginRequest struct {
	Email         string `json:"email" binding:"required,email,max=254"`
	Password      string `json:"password" binding:"required,max=72"`
	CaptchaID     string `json:"captcha_id" binding:"required,len=64,hexadecimal"`
	CaptchaAnswer string `json:"captcha_answer" binding:"required,max=3,numeric"`
}

type UserDTO struct {
	ID                string   `json:"id"`
	FirstName         string   `json:"first_name"`
	LastName          *string  `json:"last_name"`
	Photo             *string  `json:"photo"`
	PhotoAttachmentID *string  `json:"photo_attachment_id"`
	Email             string   `json:"email"`
	Roles             []string `json:"roles"`
	// Permissions are the distinct permission codes granted by the user's roles.
	Permissions []string `json:"permissions"`
}

type LoginResponse struct {
	Token     string  `json:"token"`
	TokenType string  `json:"token_type"`
	ExpiresIn int64   `json:"expires_in"`
	User      UserDTO `json:"user"`
}
