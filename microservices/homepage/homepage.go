package homepage

import (
	"net/http"

	"github.com/wk-y/rama-swap/microservices"
)

const HomepageUrl = "/"

type Homepage struct{}

var _ microservices.Microservice = (*Homepage)(nil)

func NewHomepage() *Homepage {
	return &Homepage{}
}

func (h *Homepage) HandleHomepage(w http.ResponseWriter, r *http.Request) {
	homepageTempl().Render(r.Context(), w)
}

func (h *Homepage) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc(HomepageUrl+"{$}", h.HandleHomepage)
}
