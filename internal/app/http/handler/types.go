package handler

type loginUserReq struct {
	UserToken string `json:"user_token"`
	Password  string `json:"password"`
}

type userReq struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	UserToken string `json:"user_token"`
	Password  string `json:"password"`
	IsAdmin   bool   `json:"is_admin"`
}
