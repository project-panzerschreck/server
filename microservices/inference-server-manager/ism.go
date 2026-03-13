package ism

import (
	"database/sql"
	_ "embed"
	"net/http"

	"github.com/wk-y/rama-swap/database"
	"github.com/wk-y/rama-swap/microservices"
	_ "modernc.org/sqlite"
)

type InferenceServerManager struct {
	db *sql.DB
}

type InstanceInfo struct {
	port int
}

func NewInferenceServerManager() *InferenceServerManager {
	db := database.GetDB()

	return &InferenceServerManager{
		db: db,
	}
}

// RegisterHandlers implements [microservices.Microservice].
func (s *InferenceServerManager) RegisterHandlers(mux *http.ServeMux) {
	// N/A
}

var _ microservices.Microservice = (*InferenceServerManager)(nil)
