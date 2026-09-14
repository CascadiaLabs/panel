package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
}

func (s *Store) CountUsers() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) CreateUser(username, passwordHash string) (User, error) {
	u := User{ID: mustUUID(), Username: username, PasswordHash: passwordHash}
	now := time.Now().Unix()
	u.CreatedAt, u.UpdatedAt = now, now
	_, err := s.db.Exec(`INSERT INTO users (id, username, password_hash, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		u.ID, u.Username, u.PasswordHash, u.CreatedAt, u.UpdatedAt)
	return u, err
}

func (s *Store) GetUserByUsername(username string) (User, error) {
	var u User
	err := s.db.QueryRow(`SELECT id, username, password_hash, created_at, updated_at FROM users WHERE username = ?`, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

func (s *Store) GetUserByID(id string) (User, error) {
	var u User
	err := s.db.QueryRow(`SELECT id, username, password_hash, created_at, updated_at FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

func (s *Store) UpdateUserPassword(id, passwordHash string) error {
	_, err := s.db.Exec(`UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`,
		passwordHash, time.Now().Unix(), id)
	return err
}

type Session struct {
	ID        string `json:"-"`
	UserID    string `json:"user_id"`
	CreatedAt int64  `json:"created_at"`
	ExpiresAt int64  `json:"expires_at"`
}

func (s *Store) CreateSession(userID string, ttl time.Duration) (Session, error) {
	sess := Session{
		ID:        mustToken(32),
		UserID:    userID,
		CreatedAt: time.Now().Unix(),
		ExpiresAt: time.Now().Add(ttl).Unix(),
	}
	_, err := s.db.Exec(`INSERT INTO sessions (id, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		sess.ID, sess.UserID, sess.CreatedAt, sess.ExpiresAt)
	return sess, err
}

func (s *Store) GetValidSession(id string) (Session, error) {
	var sess Session
	err := s.db.QueryRow(`SELECT id, user_id, created_at, expires_at FROM sessions WHERE id = ? AND expires_at > ?`, id, time.Now().Unix()).
		Scan(&sess.ID, &sess.UserID, &sess.CreatedAt, &sess.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sess, ErrNotFound
	}
	return sess, err
}

// ValidSession реализует auth.SessionValidator: возвращает id пользователя для валидной сессии.
func (s *Store) ValidSession(id string) (string, bool) {
	sess, err := s.GetValidSession(id)
	if err != nil {
		return "", false
	}
	return sess.UserID, true
}

func (s *Store) DeleteSession(id string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

// DeleteUserSessionsExcept удаляет все сессии пользователя, кроме указанной (текущей).
func (s *Store) DeleteUserSessionsExcept(userID, keepID string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE user_id = ? AND id <> ?`, userID, keepID)
	return err
}

func (s *Store) DeleteExpiredSessions() error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at <= ?`, time.Now().Unix())
	return err
}

// SeedAdmin создаёт пользователя admin, если таблица пользователей пуста.
// Если password пуст — генерирует случайный и возвращает его для вывода в лог.
func (s *Store) SeedAdmin(password string) (created bool, generated string, err error) {
	n, err := s.CountUsers()
	if err != nil || n > 0 {
		return false, "", err
	}
	if password == "" {
		b := make([]byte, 18)
		if _, err := rand.Read(b); err != nil {
			return false, "", err
		}
		password = base64.RawURLEncoding.EncodeToString(b)
		generated = password
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return false, "", err
	}
	if _, err := s.CreateUser("admin", string(hash)); err != nil {
		return false, "", err
	}
	return true, generated, nil
}
