package models

import "time"

type File struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Priority  int        `json:"priority"`
	ParentID  *string    `json:"parent_id"`
	S3Key     string     `json:"s3_key"`
	IsFile    bool       `json:"is_file"`
	Icon      []byte     `json:"icon"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at"`
}
