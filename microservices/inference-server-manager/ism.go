package ism

import (
	"database/sql"
	_ "embed"
	"net/http"

	"github.com/wk-y/rama-swap/microservices"
	_ "modernc.org/sqlite"
)

type InferenceServerManager struct {
	db *sql.DB
}

type InstanceInfo struct {
	port int
}

//go:embed schema.sql
var initializeSql string

func NewInferenceServerManager() *InferenceServerManager {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		panic(err)
	}

	_, err = db.Exec(initializeSql)
	if err != nil {
		panic(err)
	}

	return &InferenceServerManager{
		db: db,
	}
}

// RegisterHandlers implements [microservices.Microservice].
func (s *InferenceServerManager) RegisterHandlers(mux *http.ServeMux) {
	// N/A
}

var _ microservices.Microservice = (*InferenceServerManager)(nil)
