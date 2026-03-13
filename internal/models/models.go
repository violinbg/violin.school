// Package models defines data structures for Violin School.
package models

import "time"

// User represents an application user.
type User struct {
	ID           string
	Username     string
	FullName     string
	PasswordHash string
	Role         string
	Active       bool
	Language     string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	LastLogin    *time.Time
}

// AppConfig represents a key/value configuration entry.
type AppConfig struct {
	Key   string
	Value string
}

// Course represents a learning course on the platform.
type Course struct {
	ID          string
	Title       string
	Description string
	AuthorID    string
	Published   bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
