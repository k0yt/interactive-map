package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"interactive-map/config"
	"interactive-map/store"

	_ "github.com/lib/pq"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/gorilla/mux"
)

var (
	db           *sql.DB
	storeLayer   *store.Store
	areasCache   []store.Area
	cacheUpdated time.Time
	cacheMu      sync.RWMutex
)

func main() {
	// 1. Конфиг
	cfg := config.Load()

	// 2. Подключение к БД
	connStr := fmt.Sprintf(
		"postgres://%s:%s@%s:5432/%s?sslmode=disable",
		cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBName,
	)
	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer db.Close()

	// Ждём старта Postgres
	for i := 0; i < 30; i++ {
		if err := db.Ping(); err == nil {
			break
		}
		time.Sleep(time.Second)
	}

	// 3. Миграции
	if err := runMigrations(); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	// 4. Синхронизация areas из GeoJSON
	if err := syncAreasFromGeoJSON("./static/countries.geojson"); err != nil {
		log.Fatalf("sync areas: %v", err)
	}

	// 5. Store
	storeLayer, err = store.NewStore(db)
	if err != nil {
		log.Fatalf("init store: %v", err)
	}

	// 6. HTTP
	r := mux.NewRouter()
	r.Use(loggingMiddleware)
	r.HandleFunc("/api/areas", getAreasHandler).Methods("GET")
	r.HandleFunc("/api/users", getUsersHandler).Methods("GET")
	r.HandleFunc("/api/mark", markHandler).Methods("POST")
	r.PathPrefix("/").Handler(http.FileServer(http.Dir("./static/")))

	srv := &http.Server{Addr: ":" + cfg.HTTPPort, Handler: r}
	go func() {
		log.Printf("Server on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}



// runMigrations применяет все .up.sql из db/migrations
func runMigrations() error {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return err
	}
	m, err := migrate.NewWithDatabaseInstance(
		"file://db/migrations",
		"postgres", driver,
	)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

// syncAreasFromGeoJSON читает GeoJSON и наполняет таблицу areas,
// используя ISO3166-1-Alpha-3 из properties как строковый PK.
func syncAreasFromGeoJSON(path string) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	var fc struct {
		Features []struct {
			Properties map[string]string `json:"properties"`
		} `json:"features"`
	}
	if err := json.Unmarshal(data, &fc); err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, f := range fc.Features {
		iso3 := f.Properties["ISO3166-1-Alpha-3"]
		name := f.Properties["name"]
		if iso3 == "" || name == "" {
			continue
		}
		if _, err := tx.Exec(
			`INSERT INTO areas(id, type, name)
			 VALUES($1, 'country', $2)
			 ON CONFLICT(id) DO NOTHING`,
			iso3, name,
		); err != nil {
			return fmt.Errorf("insert area %s: %w", iso3, err)
		}
	}

	return tx.Commit()
}

// loggingMiddleware логирует HTTP-запросы
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{w, http.StatusOK}
		next.ServeHTTP(rw, r)
		log.Printf("%s %s %d %s",
			r.Method, r.RequestURI, rw.status, time.Since(start))
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// getAreasHandler возвращает список всех областей.
// Кэширует результат на 2 секунды.
func getAreasHandler(w http.ResponseWriter, r *http.Request) {
	cacheMu.RLock()
	if time.Since(cacheUpdated) < 2*time.Second {
		json.NewEncoder(w).Encode(areasCache)
		cacheMu.RUnlock()
		return
	}
	cacheMu.RUnlock()

	areas, err := storeLayer.GetAreas(r.Context())
	if err != nil {
		log.Printf("GetAreas error: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	cacheMu.Lock()
	areasCache = areas
	cacheUpdated = time.Now()
	cacheMu.Unlock()

	json.NewEncoder(w).Encode(areas)
}

// getUsersHandler возвращает список пользователей.
// Если передан параметр ?area_id=ISO3, фильтрует по этой области.
func getUsersHandler(w http.ResponseWriter, r *http.Request) {
	var areaID *string
	if s := r.URL.Query().Get("area_id"); s != "" {
		areaID = &s
	}

	users, err := storeLayer.GetUsers(r.Context(), areaID)
	if err != nil {
		log.Printf("GetUsers error: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(users)
}

// markHandler создаёт пользователя при необходимости и ставит отметку.
// Ожидает JSON {"user":"Alice","area_id":"FRA"}.
func markHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		User   string `json:"user"`
		AreaID string `json:"area_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.User == "" || req.AreaID == "" {
		http.Error(w, "user and area_id are required", http.StatusBadRequest)
		return
	}

	userID, err := storeLayer.AddUser(r.Context(), req.User)
	if err != nil {
		log.Printf("AddUser error: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if err := storeLayer.AddMark(r.Context(), userID, req.AreaID); err != nil {
		log.Printf("AddMark error: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
