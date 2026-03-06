package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"sirherobrine23.com.br/Sirherobrine23/zh-volt/olt"
)

func NewApi(maneger *olt.OltManeger) *http.ServeMux {
	Api := http.NewServeMux()

	Api.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		data := maneger.Olts()
		w.Header().Set("Content-Type", "application/json; utf-8")
		js := json.NewEncoder(w)
		js.SetIndent("", "  ")
		if err := js.Encode(data); err != nil {
			fmt.Fprintf(os.Stderr, "error on encode olt: %s\n", err)
			w.Header().Set("Content-Type", "text/plain; utf-8")
			w.WriteHeader(500)
			fmt.Fprintf(w, "error on encode olt data: %s\n\n", err)
		}
	})

	return Api
}
