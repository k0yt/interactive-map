-- db/migrations/0001_init.up.sql

-- Создаём таблицу пользователей
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    name TEXT UNIQUE NOT NULL
);

-- Создаём таблицу областей с текстовым первичным ключом ISO_A3
CREATE TABLE IF NOT EXISTS areas (
    id TEXT PRIMARY KEY,       -- ISO3166-1-Alpha-3
    type TEXT NOT NULL,        -- 'country' или 'city'
    name TEXT NOT NULL
);

-- Создаём таблицу отметок, связывающую пользователя и область
CREATE TABLE IF NOT EXISTS marks (
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    area_id TEXT NOT NULL REFERENCES areas(id) ON DELETE CASCADE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, area_id)
);

-- Индекс для ускорения подсчёта числа отметок по области
CREATE INDEX IF NOT EXISTS idx_marks_area ON marks(area_id);
