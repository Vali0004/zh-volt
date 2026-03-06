package web

import (
	"net/http"

	"sirherobrine23.com.br/Sirherobrine23/zh-volt/web/api"
	"sirherobrine23.com.br/Sirherobrine23/zh-volt/web/page"

	"sirherobrine23.com.br/Sirherobrine23/zh-volt/olt"
)

func NewWeb(maneger *olt.OltManeger) *http.ServeMux {
	web := http.NewServeMux()

	web.Handle("/api", api.NewApi(maneger))
	web.Handle("/", page.NewPage(maneger))

	return web
}
