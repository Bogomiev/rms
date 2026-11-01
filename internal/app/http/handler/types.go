package handler

type loginUserReq struct {
	UserToken string `json:"user_token"`
	Password  string `json:"password"`
}
