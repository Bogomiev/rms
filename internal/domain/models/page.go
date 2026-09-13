package models

type UserPage struct {
	Limit  int
	Offset int
}

func (p UserPage) Valid() bool { return p.Limit >= 1 && p.Limit <= 100 && p.Offset >= 0 }
