package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Area представляет область на карте с количеством отметок.
type Area struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// Store инкапсулирует работу с БД.
type Store struct {
	db             *sql.DB
	stmtGetAreas   *sql.Stmt
	stmtGetUsers   *sql.Stmt
	stmtCreateUser *sql.Stmt
	stmtInsertMark *sql.Stmt
}

// NewStore создаёт Store и готовит запросы.
func NewStore(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.prepare(); err != nil {
		return nil, fmt.Errorf("prepare statements: %w", err)
	}
	return s, nil
}

func (s *Store) prepare() error {
	var err error

	// Получить все области с числом уникальных посетителей
	s.stmtGetAreas, err = s.db.Prepare(`
		SELECT a.id, a.name, a.type, COUNT(DISTINCT m.user_id) AS cnt
		FROM areas a
		LEFT JOIN marks m ON a.id = m.area_id
		GROUP BY a.id
	`)
	if err != nil {
		return err
	}

	// Получить пользователей, опционально фильтр по area_id (TEXT)
	s.stmtGetUsers, err = s.db.Prepare(`
		SELECT DISTINCT u.name
		FROM users u
		JOIN marks m ON u.id = m.user_id
		WHERE ($1::text IS NULL OR m.area_id = $1)
	`)
	if err != nil {
		return err
	}

	// Вставка пользователя
	s.stmtCreateUser, err = s.db.Prepare(`
		INSERT INTO users(name)
		VALUES($1)
		ON CONFLICT(name) DO NOTHING
		RETURNING id
	`)
	if err != nil {
		return err
	}

	// Вставка отметки по строковому area_id
	s.stmtInsertMark, err = s.db.Prepare(`
		INSERT INTO marks(user_id, area_id)
		VALUES($1, $2)
		ON CONFLICT(user_id, area_id) DO NOTHING
	`)
	return err
}

// GetAreas возвращает все области с count отметок.
func (s *Store) GetAreas(ctx context.Context) ([]Area, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	rows, err := s.stmtGetAreas.QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Area
	for rows.Next() {
		var a Area
		if err := rows.Scan(&a.ID, &a.Name, &a.Type, &a.Count); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

// GetUsers возвращает список имён, фильтруя по строковому ISO-коду, если задан.
func (s *Store) GetUsers(ctx context.Context, areaID *string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var param interface{}
	if areaID != nil {
		param = *areaID
	}
	rows, err := s.stmtGetUsers.QueryContext(ctx, param)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		users = append(users, name)
	}
	return users, rows.Err()
}

// AddUser создаёт пользователя или возвращает уже существующий ID.
func (s *Store) AddUser(ctx context.Context, name string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var id int
	err := s.stmtCreateUser.QueryRowContext(ctx, name).Scan(&id)
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	if id == 0 {
		if err := s.db.QueryRowContext(ctx, "SELECT id FROM users WHERE name=$1", name).Scan(&id); err != nil {
			return 0, err
		}
	}
	return id, nil
}

// AddMark добавляет отметку пользователя по строковому areaID.
func (s *Store) AddMark(ctx context.Context, userID int, areaID string) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	_, err := s.stmtInsertMark.ExecContext(ctx, userID, areaID)
	return err
}
